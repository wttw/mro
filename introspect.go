package main

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/knq/snaker"
)

// In case we need to do something version specific
var dbVersion int

// Field describes a single column of a table, and is also abused to store
// query parameters
type Field struct {
	Name       string
	Position   int
	Type       string
	NotNull    bool
	Array      bool
	GoType     string
	visible    bool
	typeid     uint32
	HasDefault bool
}

// Unique describes a unique index
type Unique struct {
	Name       string
	PrimaryKey bool
	Columns    []string
}

// ForeignKey describes a foreign key
type ForeignKey struct {
	Name           string
	Columns        []string
	ForeignTable   string
	ForeignColumns []string
}

// Query describes a SQL query
type Query struct {
	Name          string
	Query         string
	OriginalQuery string
	Fields        []Field
	Parameters    []Field
	SingleRow     bool
}

// Table describes a database table
type Table struct {
	oid         uint32
	Name        string
	Schema      string
	Type        string
	Fields      []Field
	Indexes     []Unique
	Primary     Unique
	IDField     Field
	Queries     []Query
	ForeignKeys []ForeignKey
}

// Enum describes a database enum type
type Enum struct {
	oid    uint32
	Name   string
	Labels []string
}

// Result is all the information generated from database introspection
type Result struct {
	Tables []Table
	Enums  []Enum
}

// type mappings, pulled in from config
var nullType = map[uint32]string{}
var notNullType = map[uint32]string{}

// enums we've seen in a query or table OID -> name
var seenEnums = map[uint32]string{}

// all enums, extracted from pg_type OID -> name
var allEnums = map[uint32]string{}
var result Result
var queries = map[string]string{}

// introspect does all the database work needed to create our Result
// object
func introspect() Result {
	// Get the version of the database we're talking to
	var versionString string
	err := db.QueryRow(`select current_setting('server_version_num')`).Scan(&versionString)
	if err != nil {
		log.Fatalf("Failed to read server version: %s", err)
	}
	dbVersion, err = strconv.Atoi(versionString)
	if err != nil {
		log.Fatalf("Bad version '%s': %s", versionString, err)
	}

	// Get a list of enums that are in the database
	err = listEnums()
	if err != nil {
		log.Fatalf("%s", err)
	}

	// Fetch the postgresql-to-Go type maps from the configuration file
	err = readTypes()
	if err != nil {
		log.Fatalf("%s", err)
	}

	// Get the structure of tables we're interested in
	err = readTables()
	if err != nil {
		log.Fatalf("%s", err)
	}

	// Load the labels of enums we've seen in use in a table
	err = loadEnums()
	if err != nil {
		log.Fatalf("%s", err)
	}

	// Read SQL queries from configuration file
	err = readQueries()
	if err != nil {
		log.Fatalf("%s", err)
	}

	// Find unique indexes for each table, synthesize queries
	err = readIndexes()
	if err != nil {
		log.Fatalf("%s", err)
	}

	// Find foreign keys for each table, synthesize queries
	err = readFKs()
	if err != nil {
		log.Fatalf("%s", err)
	}

	// Generate types for each SQL query
	err = generateQueries()
	if err != nil {
		log.Fatalf("%s", err)
	}

	// Remove columns that aren't visible, either ignored or deleted
	removeColumns()

	// Try and avoid name clashes between parameters and internal template variables
	fixQueryParameters()

	return result
}

var nameMapping = map[string]string{}
var seenNameMapping = map[string]struct{}{}
var nonLetterRe = regexp.MustCompile(`[^\pL_]`)

// goname converts a snake_case name to a GoStyle name
// It also maps invalid names to valid ones and avoids collisions
func goname(s string) string {
	r, ok := nameMapping[s]
	if ok {
		return r
	}

	r = snaker.SnakeToCamel(nonLetterRe.ReplaceAllString(s, "_x"))

	_, ok = seenNameMapping[r]
	if ok {
		i := 2
		for {
			u := fmt.Sprintf("%s%d", r, i)
			_, ok = seenNameMapping[u]
			if !ok {
				r = u
				break
			}
			i++
		}
	}

	seenNameMapping[r] = struct{}{}
	nameMapping[s] = r
	return r
}

// makeRegexp converts a slice of glob patterns to a regexp
func makeRegexp(parts []string) string {
	for k, v := range parts {
		parts[k] = strings.Replace(regexp.QuoteMeta(v), `\*`, `.*`, -1)
	}
	return "^(" + strings.Join(parts, "|") + ")$"
}

func removeColumns() {
	for ti, table := range result.Tables {
		newFields := []Field{}
		for _, f := range table.Fields {
			if f.visible {
				newFields = append(newFields, f)
			}
		}
		table.Fields = newFields
		result.Tables[ti] = table
	}
}

// readTypes takes the type mappings from the configuration file
// sanity checks them and normalizes them
func readTypes() error {
	for k, v := range c.NotNullTypes {
		var canonicalType uint32
		if k == "*" {
			notNullType[0] = v
			continue
		}
		qerr := db.QueryRow(`select $1::regtype::oid`, k).Scan(&canonicalType)
		if qerr != nil {
			log.Printf("Failed to canonicalize type '%s': %s", k, qerr)
			continue
		}
		al, ok := notNullType[canonicalType]
		if ok {
			// We have an alias
			if al != v {
				return fmt.Errorf("Postgresql type '%s' is mapped two different ways, to '%s' and '%s'", k, v, al)
			}
		} else {
			notNullType[canonicalType] = v
		}
	}

	for k, v := range c.Types {
		var canonicalType uint32
		if k == "*" {
			nullType[0] = v
			continue
		}
		qerr := db.QueryRow(`select $1::regtype::oid`, k).Scan(&canonicalType)
		if qerr != nil {
			log.Printf("Failed to canonicalize type '%s': %s", k, qerr)
			continue
		}
		al, ok := nullType[canonicalType]
		if ok {
			// We have an alias
			if al != v {
				return fmt.Errorf("Postgresql type '%s' is mapped two different ways, to '%s' and '%s'", k, v, al)
			}
		} else {
			nullType[canonicalType] = v
		}
		// A nullable type can be used for a not null field, so fill in gaps ...
		_, ok = notNullType[canonicalType]
		if !ok {
			notNullType[canonicalType] = v
		}
	}
	return nil
}

// readTables reads all the tables
func readTables() error {
	include := c.IncludeTables
	if len(include) == 0 {
		include = []string{"public.*"}
	}
	for k, v := range include {
		if !strings.Contains(v, ".") {
			include[k] = "public." + v
		}
	}

	exclude := c.ExcludeTables
	for k, v := range exclude {
		if !strings.Contains(v, ".") {
			exclude[k] = "*." + v
		}
	}

	const tableSQL = `select c.oid, c.relkind::text, c.relname, n.nspname` +
		` from pg_class c, pg_namespace n` +
		` where c.relkind in ('r', 'v', 'm') and` +
		` n.nspname || '.' || c.relname ~ $1 and` +
		` n.nspname || '.' || c.relname !~ $2 and` +
		` n.oid = c.relnamespace`

	q, err := db.Query(tableSQL, makeRegexp(include), makeRegexp(exclude))
	if err != nil {
		return err
	}
	defer q.Close()

	for q.Next() {
		t := Table{}
		err = q.Scan(&t.oid, &t.Type, &t.Name, &t.Schema)
		if err != nil {
			return err
		}

		result.Tables = append(result.Tables, t)
	}
	q.Close()

	for k, t := range result.Tables {
		conf, ok := c.Table[t.Schema+"."+t.Name]
		if !ok {
			conf, ok = c.Table[t.Name]
		}
		if !ok {
			conf = c.Default
		}

		t.Fields, err = readColumns(t.oid, t.Name, conf)
		if err != nil {
			log.Fatalln(err)
		}
		result.Tables[k] = t
	}

	return nil
}

// goType returns the Go type that a postgresql type oid maps to
func goType(oid uint32, notnull bool, typename string, tablename string) string {
	var ok bool
	var gt string
	if notnull {
		gt, ok = notNullType[oid]
		if ok {
			return gt
		}
	}
	gt, ok = nullType[oid]
	if ok {
		return gt
	}

	enumname, ok := seenEnums[oid]
	if ok {
		return goname(enumname)
	}
	// TODO: nullable enums
	enumname, ok = allEnums[oid]
	if ok {
		seenEnums[oid] = enumname
		return goname(enumname)
	}

	if notnull {
		gt, ok = notNullType[0]
		if ok {
			log.Printf("Using fallback type for type %s in %s\n", typename, tablename)
			return gt
		}
	}

	gt, ok = nullType[0]
	if ok {
		log.Printf("Using fallback type for type %s in %s\n", typename, tablename)
		return gt
	}

	log.Printf("Couldn't translate type %s in %s\n", typename, tablename)
	return "?unknown?"
}

// readColumns reads the columns for a single table
func readColumns(oid uint32, tableName string, conf TableConfig) ([]Field, error) {
	ret := []Field{}

	// Explicitly pass NULL instead of atttypmod to format_type as we
	// don't _really_ care about max length, etc
	var attrSQL string
	if dbVersion >= 110000 {
		// Identity columns were added in 11.0, we treat them as
		// having a default
		attrSQL = `select a.attnum, a.attname, format_type(a.atttypid, NULL),` +
			` a.attnotnull, a.attndims <> 0,` +
			` a.attname ~* $2 and a.attname !~* $3 and not a.attisdropped,` +
			` a.atttypid, a.atthasdef or a.attidentity <> ''` +
			` from pg_attribute a` +
			` where a.attrelid = $1` +
			` order by a.attnum asc`
	} else {
		attrSQL = `select a.attnum, a.attname, format_type(a.atttypid, NULL),` +
			` a.attnotnull, a.attndims <> 0,` +
			` a.attname ~* $2 and a.attname !~* $3 and not a.attisdropped,` +
			` a.atttypid, a.atthasdef` +
			` from pg_attribute a` +
			` where a.attrelid = $1` +
			` order by a.attnum asc`
	}

	include := conf.IncludeColumns
	if len(include) == 0 {
		include = []string{"*"}
	}

	q, err := db.Query(attrSQL, oid, makeRegexp(include), makeRegexp(conf.ExcludeColumns))

	if err != nil {
		return nil, err
	}

	defer q.Close()
	for q.Next() {
		f := Field{}
		err = q.Scan(&f.Position, &f.Name, &f.Type, &f.NotNull, &f.Array, &f.visible, &f.typeid, &f.HasDefault)
		if err != nil {
			return nil, err
		}
		if f.Position < 0 {
			// System field
			// TODO: check for explicit match in IncludeColumns
			continue
		}

		if f.visible {
			// Only look at the type of a field if we're not ignoring it
			colType := f.Type
			if f.Array {
				colType = colType + "[]"
			}

			// table specific override
			gotype, ok := conf.ColumnType[f.Name]
			if !ok {
				gotype = goType(f.typeid, f.NotNull, colType, tableName)
			}

			f.GoType = gotype
		}
		ret = append(ret, f)
	}
	return ret, nil
}

// listEnums loads a list of all enum types
func listEnums() error {
	q, err := db.Query(`select oid, typname from pg_type where typtype = 'e'`)
	if err != nil {
		return err
	}
	defer q.Close()
	for q.Next() {
		var oid uint32
		var name string
		err = q.Scan(&oid, &name)
		if err != nil {
			return err
		}
		allEnums[oid] = name
	}
	return nil
}

// loadEnums loads the enum types that we've seen in a table
func loadEnums() error {
	for oid, name := range seenEnums {
		e := Enum{
			Name: name,
		}
		q, err := db.Query(`select enumlabel from pg_enum`+
			` where enumtypid=$1`+
			` order by enumsortorder`, oid)

		if err != nil {
			return err
		}
		defer q.Close()
		values := []string{}
		for q.Next() {
			var label string
			err = q.Scan(&label)
			if err != nil {
				return err
			}
			values = append(values, label)
		}
		e.Labels = values
		q.Close()
		result.Enums = append(result.Enums, e)
	}
	return nil
}

// readIndexes finds all the unique indexes for all our tables
func readIndexes() error {
	for k, v := range result.Tables {
		var err error
		v.Indexes, err = uniques(v)
		if err != nil {
			return err
		}
		for _, idx := range v.Indexes {
			if idx.PrimaryKey {
				v.Primary = idx
				// If there's a single column primary key we use that for update / upsert
				if len(idx.Columns) == 1 {
					for _, field := range v.Fields {
						if field.Name == idx.Columns[0] && field.HasDefault {
							v.IDField = field
						}
					}
				}
			}
			if c.GenerateUniqueQueries || (c.GeneratePKQueries && idx.PrimaryKey) {
				// Generate a query based on this index
				nameParts := []string{v.Name, "by"}
				paramParts := []string{}
				for pidx, pname := range idx.Columns {
					nameParts = append(nameParts, pname)
					paramParts = append(paramParts, fmt.Sprintf("%s = $%d", maybequote1(pname), pidx+1))
				}

				addQuery(goname(strings.Join(nameParts, "_")),
					fmt.Sprintf("select * from %s where %s", maybequote1(v.Name), strings.Join(paramParts, " and ")))
			}
		}
		result.Tables[k] = v
	}
	return nil
}

func readFKs() error {
	for k, table := range result.Tables {
		q, err := db.Query(`select conname, confrelid, conkey, confkey from pg_constraint where conrelid=$1 and contype='f'`,
			table.oid)
		if err != nil {
			return err
		}

		for q.Next() {
			fk := ForeignKey{
				Columns:        []string{},
				ForeignColumns: []string{},
			}
			conkey := []int16{}
			confkey := []int16{}
			var confrelid uint32
			err = q.Scan(&fk.Name, &confrelid, &conkey, &confkey)
			if err != nil {
				return err
			}

			if len(conkey) == 0 {
				// ??
				continue
			}
			for _, col := range conkey {
				if col < 1 || int(col) > len(table.Fields) {
					return fmt.Errorf("Column %d of foreign key %s outside columns for %s", col, fk.Name, table.Name)
				}
				fk.Columns = append(fk.Columns, table.Fields[col-1].Name)
			}

			for _, foreignTable := range result.Tables {
				if foreignTable.oid == confrelid {
					fk.ForeignTable = foreignTable.Name
					for _, fcol := range confkey {
						if fcol < 1 || int(fcol) > len(foreignTable.Fields) {
							return fmt.Errorf("Column %d of foreign key %s outside columns of target table %s", fcol, fk.Name, fk.ForeignTable)
						}
						fk.ForeignColumns = append(fk.ForeignColumns, foreignTable.Fields[fcol-1].Name)
					}
				}
			}
			result.Tables[k].ForeignKeys = append(result.Tables[k].ForeignKeys, fk)
			if c.GenerateFKQueries {
				nameParts := []string{table.Name, "by"}
				paramParts := []string{}
				for pidx, pname := range fk.Columns {
					nameParts = append(nameParts, pname)
					paramParts = append(paramParts, fmt.Sprintf("%s = $%d", maybequote1(pname), pidx+1))
				}
				addQuery(goname(strings.Join(nameParts, "_")),
					fmt.Sprintf("select * from %s where %s", maybequote1(table.Name), strings.Join(paramParts, " and ")))
			}
		}
	}
	return nil
}

// uniques finds all the unique indexes for a table
func uniques(t Table) ([]Unique, error) {
	q, err := db.Query(`select i.indisprimary, i.indkey::int2[], c.relname`+
		` from pg_index i, pg_class c`+
		` where i.indrelid = $1`+
		` and i.indisunique`+
		` and i.indexrelid = c.oid`, t.oid)

	if err != nil {
		return nil, err
	}
	defer q.Close()

	uniques := []Unique{}
OUTER:
	for q.Next() {
		u := Unique{}
		posns := []uint16{}
		err = q.Scan(&u.PrimaryKey, &posns, &u.Name)
		if err != nil {
			return nil, err
		}

		if len(posns) == 0 {
			continue
		}
		for _, pos := range posns {
			if pos == 0 {
				// Functional index
				continue OUTER
			}
			// Postgresql is 1-based, we're 0-based
			if !t.Fields[pos-1].visible {
				// Index on a column we're ignoring
				continue OUTER
			}
			u.Columns = append(u.Columns, t.Fields[pos-1].Name)
		}
		uniques = append(uniques, u)
	}
	return uniques, nil
}

// Read the user-provided SQL queries from the configuration file
func readQueries() error {
	queries = c.Queries
	return nil
}

// add a generated query, renaming it if needed to avoid clashes
func addQuery(name string, query string) {
	for {
		_, ok := queries[name]
		if !ok {
			break
		}
		name = name + "_"
	}
	queries[name] = query
}

func generateQueries() error {
	for name, query := range queries {
		err := readQuery(name, query, false)
		if err != nil {
			return err
		}
	}
	return nil
}

func readQuery(name string, query string, single bool) error {
	starre := regexp.MustCompile(`(?is)^\s*select\s+\*\s+(.*)`)
	realquery := query

	// Prepare the query, so we can get metadata about parameters and results
	prepared, err := db.Prepare(name, query)
	if err != nil {
		return fmt.Errorf("while preparing query %s: %s", name, err)
	}

	if len(prepared.FieldDescriptions) == 0 {
		return fmt.Errorf("query %s doesn't return anything", name)
	}
	tableoid := prepared.FieldDescriptions[0].Table

	tableidx := -1
	for i, t := range result.Tables {
		if t.oid == uint32(tableoid) {
			tableidx = i
		}
	}
	if tableidx == -1 {
		// TODO: generic queries
		return fmt.Errorf("query %s uses a table that's not included - not supported", name)
	}
	table := result.Tables[tableidx]

	matches := starre.FindStringSubmatch(query)
	if matches != nil {
		// it's a select * from a single table - rewrite to use concrete columns
		cols := []string{}
		for _, f := range table.Fields {
			if !f.visible {
				continue
			}
			cols = append(cols, maybequote1(f.Name))
		}
		realquery = `select ` + strings.Join(cols, ", ") + " " + matches[1]
		prepared, err = db.Prepare(name, query)
		if err != nil {
			return fmt.Errorf("while preparing query for *-expanded %s: %s", name, err)
		}
	}

	// Make sure the results will fit into the struct we already have created for table
	returnedFields := []Field{}
	for _, fd := range prepared.FieldDescriptions {
		if fd.Table != tableoid {
			// TODO: generic queries
			return fmt.Errorf("query %s returns from multiple tables - not supported", name)
		}
		f := table.Fields[fd.AttributeNumber-1]
		if f.Position != int(fd.AttributeNumber) {
			return fmt.Errorf("query %s - internal error finding columns", name)
		}
		returnedFields = append(returnedFields, f)
	}

	// Handle the $1, $2, $3 ... parameters
	parameterFields := []Field{}
	eqParameters := []string{}
	for i, paramoid := range prepared.ParameterOIDs {
		paramField := Field{}
		findNameRe := regexp.MustCompile(fmt.Sprintf(`\$%d\s*/\*\s*([^*]*[^ *])\s*\*/`, i+1))
		// Look for  annotations of the form /* name */ or /* name gotype */
		matches := findNameRe.FindStringSubmatch(query)
		if matches != nil {
			parts := regexp.MustCompile(`\s+`).Split(matches[1], -1)
			if len(parts) > 2 {
				return fmt.Errorf("query %s - couldn't understand type annotation '%s'", name, matches[1])
			}
			if len(parts) > 1 {
				paramField.GoType = parts[1]
			}
			if len(parts) > 0 {
				paramField.Name = parts[0]
			}
		}

		// Crude check for where constraints of the form <column_name>=$<digit>
		findEqualRe := regexp.MustCompile(fmt.Sprintf(`(\S+)\s*=\s*\$%d`, i+1))
		matches = findEqualRe.FindStringSubmatch(query)
		if matches != nil {
			eqParameters = append(eqParameters, matches[1])
		}
		if paramField.Name == "" {
			if matches != nil {
				if strings.Contains(matches[1], "_") {
					paramField.Name = goname(matches[1])
				} else {
					paramField.Name = matches[1]
				}
			} else {
				paramField.Name = fmt.Sprintf("p%d", i+1)
			}
		}

		if paramField.GoType == "" {
			// OK, lets try and guess based on the paramoid
			paramField.GoType = goType(uint32(paramoid), true, fmt.Sprintf("$%d", i+1), name)
		}
		parameterFields = append(parameterFields, paramField)
	}

	// If query ends in "limit 1" or includes /* singlerow */ make it return a single row
	limitre := regexp.MustCompile(`(?i)limit\s+1\s*;?\s*$`)

	if limitre.MatchString(query) || strings.Contains(query, "/* singlerow */") {
		single = true
	}

	// Deep nesting, but the number of indexes and parameters will be small
	// https://accidentallyquadratic.tumblr.com
	if !single && len(eqParameters) > 0 {
		// There's at least one parameter of the form column = parameter
		for _, index := range table.Indexes {
			columnMatches := 0
			for _, indexCol := range index.Columns {
				for _, paramCol := range eqParameters {
					if paramCol == indexCol {
						columnMatches++
					}
				}
			}
			if columnMatches == len(index.Columns) {
				// our parameters are a superset of the columns of a unique index, so we'll return one row
				single = true
				break
			}
		}
	}

	// Provide an escape hatch for when we want multiple rows
	if strings.Contains(query, "/* multirow */") {
		single = false
	}

	table.Queries = append(table.Queries, Query{
		Name:          name,
		Query:         realquery,
		OriginalQuery: query,
		Fields:        returnedFields,
		Parameters:    parameterFields,
		SingleRow:     single,
	})
	result.Tables[tableidx] = table

	return nil
}

// fixQueryParameters renames parameters so as not to clash with
// variables used in the generated code.
func fixQueryParameters() {
	rn := c.ReservedNames
	if len(rn) == 0 {
		rn = []string{"q", "row", "result", "db", "err"}
	}
	exclude := map[string]struct{}{}
	for _, name := range rn {
		exclude[name] = struct{}{}
	}

	for tableidx, table := range result.Tables {
		for queryidx, query := range table.Queries {
			for paramidx, param := range query.Parameters {
				_, ok := exclude[param.Name]
				if ok {
					// Clashes
					param.Name = param.Name + "_"
					query.Parameters[paramidx] = param
				}
			}
			table.Queries[queryidx] = query
		}
		result.Tables[tableidx] = table
	}
}
