package inventory

import "testing"

func TestDatabaseKind(t *testing.T) {
	for name, want := range map[string]string{
		"postgres:16":                                "PostgreSQL",
		"postgresql@17-main.service":                 "PostgreSQL",
		"postgres-exporter:latest":                   "",
		"mongo:8":                                    "MongoDB",
		"bitnami/mongodb:7.0":                        "MongoDB",
		"mongodb/mongodb-community-server:8.0-ubi8":  "MongoDB",
		"mongodb-community@8.0":                      "MongoDB",
		"mongod.service":                             "MongoDB",
		"MongoDB":                                    "MongoDB",
		"mongo-express":                              "",
		"mongodb-database-tools":                     "",
		"mcr.microsoft.com/mssql/server:2022-latest": "SQL Server",
		"mcr.microsoft.com/azure-sql-edge":           "SQL Server",
		"mcr.microsoft.com/mssql-tools":              "",
		"mssql-server.service":                       "SQL Server",
		"MSSQLSERVER":                                "SQL Server",
		"MSSQL$SQLEXPRESS":                           "SQL Server",
		"MSSQLFDLauncher$SQLEXPRESS":                 "",
		"SQLServerAgent":                             "",
	} {
		if got := DatabaseKind(name); got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
}
