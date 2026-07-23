package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/OpenNSW/nsw-agency/backend/internal/database"
	"github.com/OpenNSW/nsw-agency/backend/internal/user"
	"gorm.io/gorm"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "user" {
		usage()
		os.Exit(1)
	}

	switch os.Args[2] {
	case "add":
		runUserAdd(os.Args[3:])
	case "drop":
		runUserDrop()
	default:
		fmt.Fprintf(os.Stderr, "nswac: unknown command %q\n\n", os.Args[2])
		usage()
		os.Exit(1)
	}
}

// ---------- user add ----------

type cliUser struct {
	SSOID string   `json:"ssoid"`
	Name  string   `json:"name"`
	Email string   `json:"email"`
	Roles []string `json:"roles"`
}

type cliFile struct {
	Users []cliUser `json:"users"`
}

// runUserAdd handles both file-based and interactive user import.
// If --file is provided, it reads from the JSON file; otherwise it prompts interactively.
func runUserAdd(args []string) {
	fs := flag.NewFlagSet("user add", flag.ExitOnError)
	fs.Usage = usage
	filePath := fs.String("file", "", "path to users JSON import file")
	if err := fs.Parse(args); err != nil {
		fatalf("%v", err)
	}

	if *filePath != "" {
		runUserAddFromFile(*filePath)
	} else {
		runUserAddInteractive()
	}
}

func runUserAddFromFile(filePath string) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		fatalf("read file: %v", err)
	}

	var sf cliFile
	if err := json.Unmarshal(data, &sf); err != nil {
		fatalf("parse JSON: %v", err)
	}
	if len(sf.Users) == 0 {
		fmt.Println("nswac: no users found in file, nothing to do")
		return
	}

	svc := newUserService()
	inserted, err := svc.CreateBulk(toBulkInputs(sf.Users))
	if err != nil {
		fatalf("%v", err)
	}
	fmt.Printf("nswac: successfully imported %d user(s)\n", inserted)
}

func runUserAddInteractive() {
	sc := bufio.NewScanner(os.Stdin)

	name := prompt(sc, "Name: ")
	if name == "" {
		fatalf("name is required")
	}
	email := prompt(sc, "Email: ")
	if email == "" {
		fatalf("email is required")
	}
	rolesInput := prompt(sc, "Roles (comma-separated): ")

	var roles []string
	for _, r := range strings.Split(rolesInput, ",") {
		if trimmed := strings.TrimSpace(r); trimmed != "" {
			roles = append(roles, trimmed)
		}
	}
	if len(roles) == 0 {
		fatalf("at least one role is required")
	}

	svc := newUserService()
	inserted, err := svc.CreateBulk([]user.BulkInput{{Name: name, Email: email, Roles: roles}})
	if err != nil {
		fatalf("%v", err)
	}
	if inserted == 0 {
		fmt.Printf("nswac: user %q already exists — roles updated\n", email)
	} else {
		fmt.Printf("nswac: user %q created successfully\n", email)
	}
}

// ---------- user drop ----------

func runUserDrop() {
	sc := bufio.NewScanner(os.Stdin)

	email := prompt(sc, "Email of user to drop: ")
	if email == "" {
		fatalf("email is required")
	}

	svc := newUserService()
	if err := svc.DropUser(email); err != nil {
		fatalf("%v", err)
	}
	fmt.Printf("nswac: user %q dropped successfully\n", email)
}

// ---------- helpers ----------

func newUserService() *user.UserService {
	cfg, err := LoadConfig()
	if err != nil {
		fatalf("config: %v", err)
	}
	db, err := openDB(cfg.DB)
	if err != nil {
		fatalf("open database: %v", err)
	}
	return user.NewUserService(db)
}

func openDB(cfg database.Config) (*gorm.DB, error) {
	connector, err := database.NewConnector(cfg)
	if err != nil {
		return nil, err
	}
	return connector.Open()
}

func toBulkInputs(users []cliUser) []user.BulkInput {
	inputs := make([]user.BulkInput, len(users))
	for i, u := range users {
		inputs[i] = user.BulkInput{
			SSOID: u.SSOID,
			Name:  u.Name,
			Email: u.Email,
			Roles: u.Roles,
		}
	}
	return inputs
}

func prompt(sc *bufio.Scanner, label string) string {
	fmt.Print(label)
	sc.Scan()
	return strings.TrimSpace(sc.Text())
}

func usage() {
	fmt.Fprint(os.Stderr, `Usage: nswac user <command> [flags]

Commands:
  user add              Interactively add a single user and assign roles
  user add --file PATH  Import users and roles from a JSON file
  user drop             Interactively remove a user by email (also removes their role assignments)

Flags for user add:
  --file <path>   Path to users JSON file (required for file-based import)

JSON file format:
  {
    "users": [
      {
        "name": "Jane Doe",
        "email": "jane@agency.gov.au",
        "roles": ["lab_officer", "lab_manager"]
      }
    ]
  }

Environment variables:
  DB_DRIVER     sqlite or postgres (default: sqlite)
  DB_PATH       SQLite file path (default: ./agency_applications.db)
  DB_HOST       PostgreSQL host (default: localhost)
  DB_PORT       PostgreSQL port (default: 5432)
  DB_USER       PostgreSQL user (default: postgres)
  DB_PASSWORD   PostgreSQL password (required for postgres)
  DB_NAME       PostgreSQL database name (default: nsw_agency_db)
  DB_SSLMODE    PostgreSQL SSL mode (default: require)
`)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "nswac: "+format+"\n", args...)
	os.Exit(1)
}
