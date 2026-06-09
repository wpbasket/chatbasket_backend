/*
Database Migration Tool

This tool executes PostgreSQL migrations located in the 'db/common/migrations'
and 'db/personal/migrations' directories. It supports two operation modes:

1. INTERACTIVE WIZARD MODE (No command-line arguments passed)
   Prompts the user step-by-step to choose a database connection and the action to perform.
   Actions include: Run all UP, Run all DOWN, Run SELECTIVE (pick specific files),
   Inspect Database, and Database Summary.
   Usage:
     go run main.go

2. NON-INTERACTIVE CLI FLAG MODE (Command-line arguments passed)
   Runs instantly using flags. In this mode, all options are required and must be explicitly specified:
   - The '-url' flag is mandatory (can be a raw connection string or the name of an environment variable).
   - Exactly one action ('-up' or '-down') must be set to true (no default action).

   Flags:
     -url string   Connection string or env var name (e.g. DATABASE_URL_PG_TESTING). (Required)
     -up           Run all UP migrations. (Required if -down is false)
     -down         Run all DOWN migrations in reverse order. (Required if -up is false)

   Usage Examples:
     go run main.go -url="DATABASE_URL_PG_TESTING" -up
     go run main.go -url="postgres://user:pass@host:port/dbname" -up
     go run main.go -down -url="DATABASE_URL_PG_DEV"

   Note: The order of command-line flags does not matter.
*/
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
)

// ANSI color codes for premium CLI styling
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[94m"          // Bright/Light Blue for readability
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorOrange = "\033[38;5;208m" // Vibrant Orange for section titles
)

func main() {
	root := findProjectRoot()

	// Load environment variables relative to the project root
	_ = godotenv.Load(filepath.Join(root, ".env"))

	// Determine if running in non-interactive CLI mode based on command-line arguments
	if len(os.Args) > 1 {
		dbURLFlag := flag.String("url", "", "Database URL connection string or env variable name (Required)")
		runUpFlag := flag.Bool("up", false, "Run UP migrations")
		runDownFlag := flag.Bool("down", false, "Run DOWN migrations")
		flag.Parse()

		if *dbURLFlag == "" {
			log.Fatalf("%sError: -url flag is required in CLI mode%s", colorRed, colorReset)
		}

		dbURL := resolveURL(*dbURLFlag)

		if !*runUpFlag && !*runDownFlag {
			log.Fatalf("%sError: either -up or -down flag must be specified in CLI mode%s", colorRed, colorReset)
		}

		if *runUpFlag && *runDownFlag {
			log.Fatalf("%sError: cannot specify both -up and -down flags in CLI mode%s", colorRed, colorReset)
		}

		runMigrations(root, dbURL, *runUpFlag)
		return
	}

	// Load the database URL options for interactive mode
	testingURL := os.Getenv("DATABASE_URL_PG_TESTING")
	devURL := os.Getenv("DATABASE_URL_PG_DEV")
	cbURL := os.Getenv("DATABASE_URL_PG_CB")
	if cbURL == "" {
		cbURL = os.Getenv("DATABASE_URL")
	}

	reader := bufio.NewReader(os.Stdin)

	// Step 1: Select Database URL
	var dbURL string
	var dbLabel string
	for dbURL == "" {
		fmt.Printf("\n%s%s==================================================%s\n", colorBold, colorOrange, colorReset)
		fmt.Printf("%s%s            STEP 1: SELECT CONNECTION             %s\n", colorBold, colorOrange, colorReset)
		fmt.Printf("%s%s==================================================%s\n", colorBold, colorOrange, colorReset)
		fmt.Printf("  1. DATABASE_URL_PG_TESTING\n")
		fmt.Printf("  2. DATABASE_URL_PG_DEV\n")
		fmt.Printf("  3. DATABASE_URL_PG_CB\n")
		fmt.Printf("  4. Enter Custom URL or Env Var Name\n")
		fmt.Printf("\n%s%s❯ Select option (1-4): %s", colorBold, colorOrange, colorReset)

		dbChoice, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Printf("\n%sInput stream closed. Exiting.%s\n", colorYellow, colorReset)
				return
			}
			log.Fatalf("%sError reading input: %v%s", colorRed, err, colorReset)
		}
		dbChoice = strings.TrimSpace(dbChoice)

		switch dbChoice {
		case "1":
			dbURL = testingURL
			dbLabel = "CB Testing"
			if dbURL == "" {
				fmt.Printf("%sError: DATABASE_URL_PG_TESTING is empty.%s\n", colorRed, colorReset)
			}
		case "2":
			dbURL = devURL
			dbLabel = "CB Dev"
			if dbURL == "" {
				fmt.Printf("%sError: DATABASE_URL_PG_DEV is empty.%s\n", colorRed, colorReset)
			}
		case "3":
			dbURL = cbURL
			dbLabel = "CB Production"
			if dbURL == "" {
				fmt.Printf("%sError: DATABASE_URL_PG_CB is empty.%s\n", colorRed, colorReset)
			}
		case "4":
			fmt.Printf("%sEnter Custom Database URL or Env Var Name: %s", colorBold, colorReset)
			customURL, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					fmt.Printf("\n%sInput stream closed. Exiting.%s\n", colorYellow, colorReset)
					return
				}
				log.Fatalf("%sError reading input: %v%s", colorRed, err, colorReset)
			}
			customInput := strings.TrimSpace(customURL)
			dbURL = resolveURL(customInput)
			dbLabel = "Custom: " + customInput
			if dbURL == "" {
				fmt.Printf("%sError: Custom URL or Env Var cannot be empty.%s\n", colorRed, colorReset)
				dbLabel = ""
			}
		default:
			fmt.Printf("%sInvalid selection. Please choose 1, 2, 3, or 4.%s\n", colorRed, colorReset)
		}
	}

	// Step 2: Select Action
	for {
		fmt.Printf("\n\n%s%s==================================================%s\n", colorBold, colorOrange, colorReset)
		fmt.Printf("%s%s    STEP 2: SELECT ACTION  [%s%s%s%s]%s\n", colorBold, colorOrange, colorGreen, dbLabel, colorOrange, colorBold, colorReset)
		fmt.Printf("%s%s==================================================%s\n", colorBold, colorOrange, colorReset)
		fmt.Printf("  %s1.%s %sRun ALL UP migrations%s\n", colorYellow, colorReset, colorGreen, colorReset)
		fmt.Printf("  %s2.%s %sRun ALL DOWN migrations%s\n", colorYellow, colorReset, colorPurple, colorReset)
		fmt.Printf("  %s3.%s %sRun SELECTIVE migrations (pick files)%s\n", colorYellow, colorReset, colorBlue, colorReset)
		fmt.Printf("  4. Inspect Database (Detailed List)\n")
		fmt.Printf("  5. Database Summary (Object Counts)\n")
		fmt.Printf("  6. Exit\n")
		fmt.Printf("\n%s%s❯ Select option (1-6): %s", colorBold, colorOrange, colorReset)

		actionChoice, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Printf("\n%sInput stream closed. Exiting.%s\n", colorYellow, colorReset)
				return
			}
			log.Fatalf("%sError reading input: %v%s", colorRed, err, colorReset)
		}
		actionChoice = strings.TrimSpace(actionChoice)

		switch actionChoice {
		case "1":
			runMigrations(root, dbURL, true)
		case "2":
			runMigrations(root, dbURL, false)
		case "3":
			runSelectiveMigrations(root, dbURL, reader)
		case "4":
			inspectDatabase(dbURL)
		case "5":
			printDatabaseSummary(dbURL)
		case "6", "exit", "q":
			fmt.Printf("%sExiting.%s\n", colorGreen, colorReset)
			return
		default:
			fmt.Printf("%sInvalid selection. Please choose 1-6.%s\n", colorRed, colorReset)
		}
	}
}

// resolveURL checks if the input is the name of an environment variable and
// returns its value if set; otherwise, it returns the input string as-is.
func resolveURL(input string) string {
	if val := os.Getenv(input); val != "" {
		return val
	}
	return input
}

// findProjectRoot starts at the working directory and walks up until it finds
// a directory containing "go.mod" or the "db" folder.
func findProjectRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "db")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return cwd
}

func printDatabaseSummary(dbURL string) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		fmt.Printf("%sError: Failed to connect to database: %v%s\n", colorRed, err, colorReset)
		return
	}
	defer conn.Close(ctx)

	fmt.Printf("\n%s=== Database Summary ===%s\n", colorBold, colorReset)

	// 1. Count Tables
	var tableCount int
	err = conn.QueryRow(ctx, `
		SELECT COUNT(*) 
		FROM information_schema.tables 
		WHERE table_schema = 'public'
		  AND table_name NOT IN ('common_goose_db_version', 'personal_goose_db_version', 'schema_migrations');
	`).Scan(&tableCount)
	if err != nil {
		fmt.Printf("%sError counting tables: %v%s\n", colorRed, err, colorReset)
		return
	}

	// 2. Count Global Functions
	var funcCount int
	err = conn.QueryRow(ctx, `
		SELECT COUNT(*) 
		FROM information_schema.routines 
		WHERE routine_schema = 'public' 
		  AND routine_type = 'FUNCTION';
	`).Scan(&funcCount)
	if err != nil {
		fmt.Printf("%sError counting functions: %v%s\n", colorRed, err, colorReset)
		return
	}

	// 3. Count Indexes
	var indexCount int
	err = conn.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM 
			pg_class t,
			pg_class i,
			pg_index ix,
			pg_namespace n
		WHERE 
			t.oid = ix.indrelid
			AND i.oid = ix.indexrelid
			AND t.relnamespace = n.oid
			AND n.nspname = 'public'
			AND t.relkind = 'r'
			AND t.relname NOT IN ('common_goose_db_version', 'personal_goose_db_version', 'schema_migrations');
	`).Scan(&indexCount)
	if err != nil {
		fmt.Printf("%sError counting indexes: %v%s\n", colorRed, err, colorReset)
		return
	}

	// 4. Count Triggers
	var triggerCount int
	err = conn.QueryRow(ctx, `
		SELECT COUNT(DISTINCT trigger_name)
		FROM information_schema.triggers
		WHERE trigger_schema = 'public'
		  AND event_object_table NOT IN ('common_goose_db_version', 'personal_goose_db_version', 'schema_migrations');
	`).Scan(&triggerCount)
	if err != nil {
		fmt.Printf("%sError counting triggers: %v%s\n", colorRed, err, colorReset)
		return
	}

	// Print beautiful dashboard counts
	fmt.Printf("  • %s%sTables:%s %d\n", colorBold, colorCyan, colorReset, tableCount)
	fmt.Printf("  • %s%sTriggers:%s %d\n", colorBold, colorCyan, colorReset, triggerCount)
	fmt.Printf("  • %s%sIndexes:%s %d\n", colorBold, colorCyan, colorReset, indexCount)
	fmt.Printf("  • %s%sGlobal Functions:%s %d\n", colorBold, colorCyan, colorReset, funcCount)
	fmt.Printf("%s========================%s\n", colorBold, colorReset)
}

func inspectDatabase(dbURL string) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		fmt.Printf("%sError: Failed to connect to database: %v%s\n", colorRed, err, colorReset)
		return
	}
	defer conn.Close(ctx)

	fmt.Printf("\n%s=== Database Schema Inspection ===%s\n", colorBold, colorReset)

	// 1. Fetch triggers and group them by table name -> trigger name -> list of events
	triggerRows, err := conn.Query(ctx, `
		SELECT DISTINCT trigger_name, event_object_table
		FROM information_schema.triggers
		WHERE event_object_table NOT IN ('common_goose_db_version', 'personal_goose_db_version', 'schema_migrations')
		ORDER BY event_object_table, trigger_name;
	`)
	triggers := make(map[string][]string)
	if err == nil {
		for triggerRows.Next() {
			var name, table string
			if err := triggerRows.Scan(&name, &table); err == nil {
				triggers[table] = append(triggers[table], name)
			}
		}
		triggerRows.Close()
	}

	// 2. Fetch indexes and group them by table name
	indexRows, err := conn.Query(ctx, `
		SELECT 
			t.relname AS table_name,
			i.relname AS index_name,
			ix.indisunique AS is_unique
		FROM 
			pg_class t,
			pg_class i,
			pg_index ix,
			pg_namespace n
		WHERE 
			t.oid = ix.indrelid
			AND i.oid = ix.indexrelid
			AND t.relnamespace = n.oid
			AND n.nspname = 'public'
			AND t.relkind = 'r'
			AND t.relname NOT IN ('common_goose_db_version', 'personal_goose_db_version', 'schema_migrations')
		ORDER BY 
			t.relname, i.relname;
	`)
	indexes := make(map[string][]string)
	if err == nil {
		for indexRows.Next() {
			var tableName, indexName string
			var isUnique bool
			if err := indexRows.Scan(&tableName, &indexName, &isUnique); err == nil {
				suffix := ""
				if isUnique {
					suffix = " (UNIQUE)"
				}
				indexes[tableName] = append(indexes[tableName], fmt.Sprintf("%s%s", indexName, suffix))
			}
		}
		indexRows.Close()
	}

	// 3. Fetch and print Tables (nested with their triggers and indexes)
	fmt.Printf("\n%s%sTables, Triggers & Indexes:%s\n", colorBold, colorCyan, colorReset)
	tableRows, err := conn.Query(ctx, `
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = 'public' 
		  AND table_name NOT IN ('common_goose_db_version', 'personal_goose_db_version', 'schema_migrations')
		ORDER BY table_name;
	`)
	if err != nil {
		fmt.Printf("%sError querying tables: %v%s\n", colorRed, err, colorReset)
	} else {
		hasTables := false
		for tableRows.Next() {
			var tableName string
			if err := tableRows.Scan(&tableName); err == nil {
				fmt.Printf("  - %s%s%s%s\n", colorBold, colorCyan, tableName, colorReset)
				hasTables = true

				tableTriggers, existsTriggers := triggers[tableName]
				tableIndexes, existsIndexes := indexes[tableName]

				// Print triggers
				if existsTriggers {
					sort.Strings(tableTriggers)
					for _, tName := range tableTriggers {
						// If there are also indexes to follow, use ├─, otherwise use └─ at the end
						prefix := "    ├─"
						if !existsIndexes && tName == tableTriggers[len(tableTriggers)-1] {
							prefix = "    └─"
						}
						fmt.Printf("%s Trigger: %s\n", prefix, tName)
					}
				}

				// Print indexes
				if existsIndexes {
					for i, idxName := range tableIndexes {
						prefix := "    ├─"
						if i == len(tableIndexes)-1 {
							prefix = "    └─"
						}
						fmt.Printf("%s Index: %s\n", prefix, idxName)
					}
				}
			}
		}
		tableRows.Close()
		if !hasTables {
			fmt.Printf("  %s(No tables found)%s\n", colorYellow, colorReset)
		}
	}

	// 4. Fetch and print Global Functions
	fmt.Printf("\n%s%sGlobal Functions:%s\n", colorBold, colorYellow, colorReset)
	funcRows, err := conn.Query(ctx, `
		SELECT routine_name
		FROM information_schema.routines
		WHERE routine_schema = 'public' 
		  AND routine_type = 'FUNCTION'
		ORDER BY routine_name;
	`)
	if err != nil {
		fmt.Printf("%sError querying functions: %v%s\n", colorRed, err, colorReset)
	} else {
		hasFuncs := false
		for funcRows.Next() {
			var funcName string
			if err := funcRows.Scan(&funcName); err == nil {
				fmt.Printf("  - %s%s()%s\n", colorGreen, funcName, colorReset)
				hasFuncs = true
			}
		}
		funcRows.Close()
		if !hasFuncs {
			fmt.Printf("  %s(No functions found)%s\n", colorYellow, colorReset)
		}
	}
	fmt.Printf("\n%s==================================%s\n", colorBold, colorReset)
}

func runSelectiveMigrations(root string, dbURL string, reader *bufio.Reader) {
	dirs := []string{"db/common/migrations", "db/personal/migrations"}

	// Step A: Choose UP or DOWN
	fmt.Printf("\n%s%s--- Selective Migration Mode ---%s\n", colorBold, colorBlue, colorReset)
	fmt.Printf("  %s1.%s Run selected %sUP%s files\n", colorYellow, colorReset, colorGreen, colorReset)
	fmt.Printf("  %s2.%s Run selected %sDOWN%s files\n", colorYellow, colorReset, colorPurple, colorReset)
	fmt.Printf("\n%s%s❯ Select direction (1-2): %s", colorBold, colorOrange, colorReset)

	dirChoice, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("%sError reading input: %v%s\n", colorRed, err, colorReset)
		return
	}
	dirChoice = strings.TrimSpace(dirChoice)

	var suffix string
	var isUp bool
	switch dirChoice {
	case "1":
		suffix = "*.up.sql"
		isUp = true
	case "2":
		suffix = "*.down.sql"
		isUp = false
	default:
		fmt.Printf("%sInvalid selection.%s\n", colorRed, colorReset)
		return
	}

	// Step B: Collect all migration files from all folders with numbering
	type migrationFile struct {
		category string
		path     string
		name     string
	}
	var allFiles []migrationFile

	for _, dir := range dirs {
		category := filepath.Base(filepath.Dir(dir))
		dirPath := filepath.Join(root, dir)
		files, _ := filepath.Glob(filepath.Join(dirPath, suffix))
		if len(files) == 0 {
			continue
		}
		if isUp {
			sort.Strings(files)
		} else {
			sort.Slice(files, func(i, j int) bool {
				return files[i] > files[j]
			})
		}
		for _, f := range files {
			allFiles = append(allFiles, migrationFile{
				category: category,
				path:     f,
				name:     filepath.Base(f),
			})
		}
	}

	if len(allFiles) == 0 {
		fmt.Printf("%sNo migration files found.%s\n", colorYellow, colorReset)
		return
	}

	// Step C: Display numbered list grouped by folder
	dirLabel := "UP"
	dirColor := colorGreen
	if !isUp {
		dirLabel = "DOWN"
		dirColor = colorPurple
	}

	fmt.Printf("\n%s%s--- Available %s Files ---%s\n", colorBold, dirColor, dirLabel, colorReset)
	currentCategory := ""
	for i, mf := range allFiles {
		if mf.category != currentCategory {
			currentCategory = mf.category
			categoryName := strings.ToUpper(currentCategory[:1]) + currentCategory[1:]
			fmt.Printf("\n  %s%s[%s]%s\n", colorBold, colorCyan, categoryName, colorReset)
		}
		fmt.Printf("    %s%d.%s %s\n", colorYellow, i+1, colorReset, mf.name)
	}

	// Step D: Get user selection
	fmt.Printf("\n%s%s❯ Enter file numbers (comma-separated, e.g. 1,3,5): %s", colorBold, colorOrange, colorReset)
	selection, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("%sError reading input: %v%s\n", colorRed, err, colorReset)
		return
	}
	selection = strings.TrimSpace(selection)
	if selection == "" {
		fmt.Printf("%sNo files selected. Aborting.%s\n", colorYellow, colorReset)
		return
	}

	// Parse selected numbers
	parts := strings.Split(selection, ",")
	var selected []migrationFile
	for _, p := range parts {
		p = strings.TrimSpace(p)
		var num int
		_, err := fmt.Sscanf(p, "%d", &num)
		if err != nil || num < 1 || num > len(allFiles) {
			fmt.Printf("%sInvalid number '%s'. Aborting.%s\n", colorRed, p, colorReset)
			return
		}
		selected = append(selected, allFiles[num-1])
	}

	// Step E: Confirm
	fmt.Printf("\n%s%sWill run %d %s file(s):%s\n", colorBold, dirColor, len(selected), dirLabel, colorReset)
	for _, mf := range selected {
		categoryName := strings.ToUpper(mf.category[:1]) + mf.category[1:]
		fmt.Printf("  → %s[%s]%s %s\n", colorCyan, categoryName, colorReset, mf.name)
	}
	fmt.Printf("\n%s%s❯ Confirm? (y/n): %s", colorBold, colorOrange, colorReset)
	confirm, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("%sError reading input: %v%s\n", colorRed, err, colorReset)
		return
	}
	confirm = strings.TrimSpace(strings.ToLower(confirm))
	if confirm != "y" && confirm != "yes" {
		fmt.Printf("%sAborted.%s\n", colorYellow, colorReset)
		return
	}

	// Step F: Execute
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		fmt.Printf("%sError: Failed to connect to database: %v%s\n", colorRed, err, colorReset)
		return
	}
	defer conn.Close(ctx)

	for _, mf := range selected {
		categoryName := strings.ToUpper(mf.category[:1]) + mf.category[1:]
		fmt.Printf("%s[%s] Applying: %s...%s\n", dirColor, categoryName, mf.name, colorReset)

		content, err := os.ReadFile(mf.path)
		if err != nil {
			log.Printf("%sError: Failed to read file %s: %v%s\n", colorRed, mf.path, err, colorReset)
			return
		}

		_, err = conn.Exec(ctx, string(content))
		if err != nil {
			log.Printf("%sError: Failed to execute SQL in %s: %v%s\n", colorRed, mf.name, err, colorReset)
			return
		}
		fmt.Printf("%s[%s] Successfully applied: %s%s\n", colorGreen, categoryName, mf.name, colorReset)
	}
	fmt.Printf("\n%s%sSelective %s migrations completed successfully.%s\n", colorBold, colorGreen, dirLabel, colorReset)
}

func runMigrations(root string, dbURL string, isUp bool) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		fmt.Printf("%sError: Failed to connect to database: %v%s\n", colorRed, err, colorReset)
		return
	}
	defer conn.Close(ctx)

	if isUp {
		for _, dir := range []string{"db/common/migrations", "db/personal/migrations"} {
			category := filepath.Base(filepath.Dir(dir)) // "common" or "personal"
			categoryName := strings.ToUpper(category[:1]) + category[1:]
			dirPath := filepath.Join(root, dir)
			files, _ := filepath.Glob(filepath.Join(dirPath, "*.up.sql"))
			if len(files) == 0 {
				continue
			}
			sort.Strings(files)

			fmt.Printf("\n%s--- %s Migrations ---%s\n", colorBold, categoryName, colorReset)

			for _, file := range files {
				name := filepath.Base(file)
				fmt.Printf("%s[UP] Applying migration: %s...%s\n", colorCyan, name, colorReset)

				content, err := os.ReadFile(file)
				if err != nil {
					log.Printf("%sError: Failed to read file %s: %v%s\n", colorRed, file, err, colorReset)
					return
				}

				_, err = conn.Exec(ctx, string(content))
				if err != nil {
					log.Printf("%sError: Failed to execute SQL in %s: %v%s\n", colorRed, name, err, colorReset)
					return
				}
				fmt.Printf("%s[UP] Successfully applied: %s%s\n", colorGreen, name, colorReset)
			}
		}
		fmt.Printf("\n%s%sUP migrations completed successfully.%s\n", colorBold, colorGreen, colorReset)
	} else {
		for _, dir := range []string{"db/personal/migrations", "db/common/migrations"} {
			category := filepath.Base(filepath.Dir(dir)) // "personal" or "common"
			categoryName := strings.ToUpper(category[:1]) + category[1:]
			dirPath := filepath.Join(root, dir)
			files, _ := filepath.Glob(filepath.Join(dirPath, "*.down.sql"))
			if len(files) == 0 {
				continue
			}
			sort.Slice(files, func(i, j int) bool {
				return files[i] > files[j]
			})

			fmt.Printf("\n%s--- Reverting %s Migrations ---%s\n", colorBold, categoryName, colorReset)

			for _, file := range files {
				name := filepath.Base(file)
				fmt.Printf("%s[DOWN] Reverting migration: %s...%s\n", colorYellow, name, colorReset)

				content, err := os.ReadFile(file)
				if err != nil {
					log.Printf("%sError: Failed to read file %s: %v%s\n", colorRed, file, err, colorReset)
					return
				}

				_, err = conn.Exec(ctx, string(content))
				if err != nil {
					log.Printf("%sError: Failed to execute SQL in %s: %v%s\n", colorRed, name, err, colorReset)
					return
				}
				fmt.Printf("%s[DOWN] Successfully reverted: %s%s\n", colorPurple, name, colorReset)
			}
		}
		fmt.Printf("\n%s%sDOWN migrations completed successfully.%s\n", colorBold, colorGreen, colorReset)
	}
}
