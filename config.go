package main

// TableConfig holds the configuration for a single table
type TableConfig struct {
	IncludeColumns []string
	ExcludeColumns []string
	ColumnType     map[string]string
	Rename         string
}

// Config holds all configuration, as serialized from mro.cfg
type Config struct {
	ConnectionString      string
	IncludeTables         []string
	ExcludeTables         []string
	Default               TableConfig
	Table                 map[string]TableConfig
	Types                 map[string]string
	NotNullTypes          map[string]string
	JsonOutput            string
	EnumFilename          string
	EnumTemplate          string
	TableFilename         string
	TableTemplate         string
	TemplateParameters    map[string]interface{}
	GeneratePKQueries     bool
	GenerateUniqueQueries bool
	GenerateFKQueries     bool
	Queries               map[string]string
	ReservedNames         []string
	PostProcess           []string
}
