# mro
Introspect a PostgreSQL database to generate Go marshalling boilerplate

## Installation

`go get -u github.com/wttw/mro`

## Usage

`mro --bootstrap pgx` will create four files in the current directory, suitable for use with [pgx](https://github.com/jackc/pgx).

`mro.cfg` is a [HCL](https://github.com/hashicorp/hcl) format configuration file. It's hopefully self-documenting.
If nothing else you'll need to edit the ConnectionString setting to point at the database containing the schema
you want to work from.

`pgx.go` specifies the interface that mro generated code will use to access the database. It's implemented by
pgx.Conn, pgx.ConnPool and pgx.Tx.

`table.pgx.tpl` and `enum.pgx.tpl` are Go format [templates](https://golang.org/pkg/text/template/) used to generate
code.

`mro` or `mro -package <packagename>` will generate marshaling and unmarshaling code for the database schema.
For each table it will generate a struct that represents a row of the table, with a name based on the name
of the table converted into PascalCase: a table called "email_source" will map on to a struct called
"EmailSource". That struct has an Insert() method and, if there's a single-column primary key, Update(),
Upsert() and Delete() methods.

Functions to retrieve data from each table are also created. AllEmailSource() will return the entire table,
and functions named like EmailSourceByID() will be created for each primary key or unique index on the table.

Additional SQL queries can be added to the Queries section of the configuration file. These must retrieve
columns from a single table, and will generate functions to retrieve those as slices of that table's struct.

It will also generate `mro.json` containing all the information retrieved from the database, as is passed
in to the templates to generate code.

### Not supported

Any database other than PostgreSQL.

Marshaling of arbitrary SQL queries. This could be added fairly simply, but I've not needed it so far.

Any use of, for example, foreign keys to record structure at a higher level than a table.

Anything ORM-ish, beyond basic marshaling code. If you're looking for programatic generation of SQL
queries or automatic loading of related rows this isn't the best place to start.

### Known bugs

Table or column names that use unicode characters outside the basic multilingual plane aren't handled correctly.

### Other issues

This is mostly untested code. I'm using it in a large production-grade project, so I'll be dealing with bugs (and
maybe adding regression test) as I come across them.

It's also a fairly quick hack, so while the code quality isn't terrible it's definitely not great. Global variables,
everything lives in main ...

### Similar libraries

[gnorm](https://gnorm.org)

[xo](https://github.com/knq/xo)

[sqlboiler](https://github.com/volatiletech/sqlboiler) - much more ORMy