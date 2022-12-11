/*
Copyright © 2022 Ian Sinnott
*/
package cmd

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

// info from sqlite_master
type SchemaData struct {
	Type string // table, index, view, trigger
	Name string
	Sql  string
}

type TableInfo struct {
	Name    string
	Sql     string
	PkField string
	Fields  []TableFieldInfo
}

// info from PRAGMA table_info
type TableFieldInfo struct {
	Cid       int
	Name      string
	DataType  string
	NotNull   bool
	DfltValue interface{}
	Pk        bool
}

func GetTables(db *sql.DB) ([]TableInfo, error) {
	rows, err := db.Query("SELECT name,sql FROM sqlite_master WHERE type='table'")
	if err != nil {
		return nil, errors.Wrap(err, "failed to get tables")
	}

	var tables []TableInfo
	for rows.Next() {
		var tableInfo TableInfo
		err = rows.Scan(&tableInfo.Name, &tableInfo.Sql)
		if err != nil {
			return nil, errors.Wrap(err, "failed to scan table info")
		}

		rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableInfo.Name))
		if err != nil {
			return nil, errors.Wrap(err, "failed to get table info")
		}

		for rows.Next() {
			var field TableFieldInfo
			var pk int
			err = rows.Scan(&field.Cid, &field.Name, &field.DataType, &field.NotNull, &field.DfltValue, &pk)
			field.Pk = pk == 1 // convert to bool
			if err != nil {
				return nil, errors.Wrap(err, "failed to scan table field info")
			}
			tableInfo.Fields = append(tableInfo.Fields, field)
			if field.Pk {
				tableInfo.PkField = field.Name
			}
		}

		tables = append(tables, tableInfo)
	}

	return tables, nil
}

// syncCmd represents the sync command
var syncCmd = &cobra.Command{
	Use:   "sync source destination",
	Short: "A brief description of your command",
	Long: `Sync the common tables of <source> and <destination>. Tables that are
	not present in both databases are ignored. Syncing uses a simple last-write
	wins strategy, which means that each table to be synced needs to have a field
	to determine when it was updated.`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		// updatedField, err := cmd.Flags().GetString("updated-field")
		// if err != nil {
		// 	fmt.Println(err, "failed to get updated-field flag")
		// 	os.Exit(1)
		// }

		source, destination := args[0], args[1]

		// make sure the source and destination paths exists
		for _, path := range []string{source, destination} {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				fmt.Printf("path '%s' does not exist", path)
				os.Exit(1)
			}
		}

		connA, err := sql.Open("sqlite", source)
		if err != nil {
			fmt.Println(err, "failed to open source database")
			os.Exit(1)
		}
		defer connA.Close()
		connB, err := sql.Open("sqlite", destination)
		if err != nil {
			fmt.Println(err, "failed to open destination database")
			os.Exit(1)
		}
		defer connB.Close()

		tablesA, err := GetTables(connA)
		if err != nil {
			fmt.Println(err, "failed to get tables from source database")
			os.Exit(1)
		}

		tablesB, err := GetTables(connB)
		if err != nil {
			fmt.Println(err, "failed to get tables from destination database")
			os.Exit(1)
		}

		var common, uncommon []string
		validTables := []TableInfo{}
		invalidTables := []TableInfo{}
		for _, tableA := range tablesA {
			found := false
			for _, tableB := range tablesB {
				if tableA.Name == tableB.Name {
					found = true
					break
				}
			}
			if found {
				common = append(common, tableA.Name)
			} else {
				uncommon = append(uncommon, tableA.Name)
			}

			// hasUpdateField := false
			// for _, field := range tableA.Fields {
			// 	if field.Name == updatedField {
			// 		hasUpdateField = true
			// 		break
			// 	}
			// }

			valid := /* hasUpdateField && */ tableA.PkField != ""

			if valid {
				validTables = append(validTables, tableA)
			} else {
				invalidTables = append(invalidTables, tableA)
			}
		}

		if len(uncommon) > 0 {
			fmt.Println("WARN: some tables are not present in both databases. Uncommon tables will be ignored")
		}

		fmt.Println("common tables:")
		for _, table := range common {
			fmt.Println("\t", table)
		}

		fmt.Println("uncommon tables:")
		for _, table := range uncommon {
			fmt.Println("\t", table)
		}

		fmt.Println("valid tables:")
		for _, table := range validTables {
			fmt.Println("\t", table.Name)
		}

		fmt.Println("invalid tables:")
		for _, table := range invalidTables {
			fmt.Println("\t", table.Name)
		}

		for _, table := range validTables {
			fmt.Println("syncing table", table.Name)
			err := SyncTable(connA, connB, table)
			if err != nil {
				fmt.Println(err, "failed to sync table", table.Name)
				os.Exit(1)
			}
		}
	},
}

func SyncTable(connA, connB *sql.DB, table TableInfo) error {
	fmt.Println("syncing table", table.Name)

	// if table does not exist in destination, create it
	_, err := connB.Query("SELECT * FROM " + table.Name)
	if err != nil {
		fmt.Println("table does not exist in destination, creating it")
		_, err = connB.Exec(table.Sql)
		if err != nil {
			return errors.Wrap(err, "failed to create table in destination")
		}
	}

	// get the last updated time from the destination
	// var lastUpdated time.Time
	// err = connB.QueryRow("SELECT MAX(updated_at) FROM " + table.Name).Scan(&lastUpdated)
	// if err != nil {
	// 	return errors.Wrap(err, "failed to get last updated time")
	// }

	// get all records from the source that have been updated since the last update
	// _, err = connA.Query("SELECT * FROM "+table.Name+" WHERE updated_at > ?", lastUpdated)
	// if err != nil {
	// 	return errors.Wrap(err, "failed to get updated records")
	// }

	// get the primary key field
	pkField := table.PkField

	// get the fields that are not the primary key
	fields := []string{}
	for _, field := range table.Fields {
		if field.Name != pkField {
			fields = append(fields, field.Name)
		}
	}

	// @note I never finished this because there was no pressing need. it was a
	// curiosity project and the more I got into it the more it seemed it would
	// have too many caveats to be productive. For instance, how do you sync a
	// many to many relation table? These tables often don't have an updated
	// field, and adding one just to use this tool seems like a big asterisk.
	// Normally the models on either side would have the4 updated field. So how do
	// you resolve conflicts? Last write wins requires knowing the last write.

	// TODO ===========================================
	// @todo Seomthing like this: https://stackoverflow.com/questions/40177912/how-can-i-sync-two-sqlite-tables-using-sql
	// TODO ===========================================

	return nil
}

func init() {
	syncCmd.Flags().String("updated-field", "updated_at", "The name of the field that will be used to determine if a record has been updated")
	rootCmd.AddCommand(syncCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// syncCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// syncCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
