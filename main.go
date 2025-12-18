package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

var db *sql.DB

// ---------------- MASTER CONFIG ----------------

type MasterConfig struct {
	TableName string
	Fields    []string
}

// Hooks untuk tiap master
type MasterHook struct {
	BeforeCreate func(data map[string]interface{}) error
	BeforeUpdate func(data map[string]interface{}) error
	BeforeDelete func(id string) error
}

// Master tables
var masterTables = map[string]MasterConfig{
	"user": {
		TableName: "user_pengguna",
		Fields:    []string{"id", "kode", "nama", "tanggal_lahir", "lokasi", "email", "created_at", "updated_at", "deleted_at"},
	},
	"customer": {
		TableName: "customer",
		Fields:    []string{"id", "kode", "nama", "email", "telepon", "created_at", "updated_at", "deleted_at"},
	},
	"produk": {
		TableName: "produk",
		Fields:    []string{"id", "kode", "nama", "harga", "stok", "created_at", "updated_at", "deleted_at"},
	},
}

// Hooks per master
var masterHooks = map[string]MasterHook{
	"user": {
		BeforeCreate: func(data map[string]interface{}) error {
			if data["nama"] == "" {
				return fmt.Errorf("nama wajib diisi")
			}
			if data["email"] == "" {
				return fmt.Errorf("email wajib diisi")
			}
			return nil
		},
		BeforeDelete: func(id string) error {
			if id == "1" {
				return fmt.Errorf("user admin tidak bisa dihapus")
			}
			return nil
		},
	},
	"customer": {
		BeforeCreate: func(data map[string]interface{}) error {
			if data["nama"] == "" {
				return fmt.Errorf("nama wajib diisi")
			}
			return nil
		},
	},
	"produk": {
		BeforeCreate: func(data map[string]interface{}) error {
			if data["nama"] == "" {
				return fmt.Errorf("nama produk wajib diisi")
			}
			if data["harga"] == nil {
				return fmt.Errorf("harga wajib diisi")
			}
			return nil
		},
	},
}

// ---------------- MAIN ----------------

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	var err error
	db, err = sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("DB connection error:", err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		log.Fatal("DB ping error:", err)
	}

	http.HandleFunc("/api/", masterHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("Running on port:", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// ---------------- HANDLER ----------------

func masterHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "api" {
		http.NotFound(w, r)
		return
	}

	masterKey := parts[1]
	config, ok := masterTables[masterKey]
	if !ok {
		http.Error(w, "Master table not found", 404)
		return
	}

	id := ""
	if len(parts) >= 3 {
		id = parts[2]
	}

	switch r.Method {
	case "GET":
		handleGet(w, r, masterKey, config, id)
	case "POST":
		handleCreate(w, r, masterKey, config)
	case "PUT":
		if id == "" {
			http.Error(w, "ID required", 400)
			return
		}
		handleUpdate(w, r, masterKey, config, id)
	case "DELETE":
		if id == "" {
			http.Error(w, "ID required", 400)
			return
		}
		handleDelete(w, r, masterKey, config, id)
	default:
		http.Error(w, "Method not allowed", 405)
	}
}

// ---------------- CRUD HANDLERS ----------------

func handleGet(w http.ResponseWriter, r *http.Request, masterKey string, config MasterConfig, id string) {
	if id != "" {
		// GET single
		var columns = strings.Join(config.Fields, ", ")
		row := db.QueryRow(fmt.Sprintf("SELECT %s FROM %s WHERE id=$1 AND deleted_at IS NULL", columns, config.TableName), id)
		data := make(map[string]interface{})
		dest := make([]interface{}, len(config.Fields))
		for i := range config.Fields {
			var tmp interface{}
			dest[i] = &tmp
		}
		if err := row.Scan(dest...); err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "Not found", 404)
				return
			}
			http.Error(w, err.Error(), 500)
			return
		}
		for i, f := range config.Fields {
			data[f] = *(dest[i].(*interface{}))
		}
		jsonResponse(w, data)
		return
	}

	// GET list
	page, limit, offset := getPagination(r)
	search := r.URL.Query().Get("q")

	query := fmt.Sprintf("SELECT %s FROM %s WHERE deleted_at IS NULL", strings.Join(config.Fields, ", "), config.TableName)
	args := []interface{}{}

	if search != "" {
		searchCond := []string{}
		for _, f := range config.Fields {
			if f == "id" || f == "created_at" || f == "updated_at" || f == "deleted_at" {
				continue
			}
			searchCond = append(searchCond, fmt.Sprintf("%s ILIKE $%d", f, len(args)+1))
			args = append(args, "%"+search+"%")
		}
		if len(searchCond) > 0 {
			query += " AND (" + strings.Join(searchCond, " OR ") + ")"
		}
	}

	query += fmt.Sprintf(" ORDER BY id DESC LIMIT %d OFFSET %d", limit, offset)

	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		dest := make([]interface{}, len(config.Fields))
		for i := range config.Fields {
			var tmp interface{}
			dest[i] = &tmp
		}
		if err := rows.Scan(dest...); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		rowMap := make(map[string]interface{})
		for i, f := range config.Fields {
			rowMap[f] = *(dest[i].(*interface{}))
		}
		results = append(results, rowMap)
	}

	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE deleted_at IS NULL", config.TableName)
	_ = db.QueryRow(countQuery).Scan(&total)

	resp := map[string]interface{}{
		"page":       page,
		"limit":      limit,
		"total_data": total,
		"total_page": (total + limit - 1) / limit,
		"data":       results,
	}
	jsonResponse(w, resp)
}

func handleCreate(w http.ResponseWriter, r *http.Request, masterKey string, config MasterConfig) {
	var data map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	// BeforeCreate hook
	if hook, ok := masterHooks[masterKey]; ok && hook.BeforeCreate != nil {
		if err := hook.BeforeCreate(data); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
	}

	fields := []string{}
	values := []interface{}{}
	placeholders := []string{}
	i := 1
	for _, f := range config.Fields {
		if f == "id" || f == "created_at" || f == "updated_at" || f == "deleted_at" {
			continue
		}
		if v, ok := data[f]; ok {
			fields = append(fields, f)
			values = append(values, v)
			placeholders = append(placeholders, fmt.Sprintf("$%d", i))
			i++
		}
	}

	fields = append(fields, "created_at", "updated_at")
	values = append(values, time.Now(), time.Now())
	placeholders = append(placeholders, fmt.Sprintf("$%d", i), fmt.Sprintf("$%d", i+1))

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING id", config.TableName, strings.Join(fields, ", "), strings.Join(placeholders, ", "))
	var lastID int64
	err := db.QueryRow(query, values...).Scan(&lastID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	jsonResponse(w, map[string]interface{}{"message": "Created successfully", "id": lastID})
}

func handleUpdate(w http.ResponseWriter, r *http.Request, masterKey string, config MasterConfig, id string) {
	var data map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	// BeforeUpdate hook
	if hook, ok := masterHooks[masterKey]; ok && hook.BeforeUpdate != nil {
		if err := hook.BeforeUpdate(data); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
	}

	fields := []string{}
	values := []interface{}{}
	i := 1
	for _, f := range config.Fields {
		if f == "id" || f == "created_at" || f == "updated_at" || f == "deleted_at" {
			continue
		}
		if v, ok := data[f]; ok {
			fields = append(fields, fmt.Sprintf("%s=$%d", f, i))
			values = append(values, v)
			i++
		}
	}

	if len(fields) == 0 {
		http.Error(w, "No fields to update", 400)
		return
	}

	fields = append(fields, fmt.Sprintf("updated_at=$%d", i))
	values = append(values, time.Now())
	i++
	values = append(values, id)

	query := fmt.Sprintf("UPDATE %s SET %s WHERE id=$%d", config.TableName, strings.Join(fields, ", "), i)
	_, err := db.Exec(query, values...)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	jsonResponse(w, map[string]interface{}{"message": "Updated successfully"})
}

func handleDelete(w http.ResponseWriter, r *http.Request, masterKey string, config MasterConfig, id string) {
	// BeforeDelete hook
	if hook, ok := masterHooks[masterKey]; ok && hook.BeforeDelete != nil {
		if err := hook.BeforeDelete(id); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
	}

	query := fmt.Sprintf("UPDATE %s SET deleted_at=$1 WHERE id=$2", config.TableName)
	_, err := db.Exec(query, time.Now(), id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	jsonResponse(w, map[string]interface{}{"message": "Deleted successfully"})
}

// ---------------- HELPERS ----------------

func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func getPagination(r *http.Request) (page, limit, offset int) {
	page, limit = 1, 10
	q := r.URL.Query()

	if p, err := strconv.Atoi(q.Get("page")); err == nil && p > 0 {
		page = p
	}
	if l, err := strconv.Atoi(q.Get("limit")); err == nil && l > 0 {
		if l > 100 {
			l = 100
		}
		limit = l
	}
	offset = (page - 1) * limit
	return
}
