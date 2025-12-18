package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

/* =========================
   GLOBAL DB
========================= */

var db *sql.DB

// CONFIG MASTER TABLE
type MasterConfig struct {
	TableName string
	Fields    []string
}

// Daftar master table yang terdaftar
var masterTables = map[string]MasterConfig{
	"user": {
		TableName: "user_pengguna",
		Fields:    []string{"id", "kode", "nama", "tanggal_lahir", "lokasi", "email", "created_at", "updated_at"},
	},
	"produk": {
		TableName: "produk",
		Fields:    []string{"id", "kode", "nama", "harga", "stok", "created_at", "updated_at"},
	},
	"customer": {
		TableName: "customer",
		Fields:    []string{"id", "kode", "nama", "email", "telepon", "created_at", "updated_at"},
	},
}

// Tambahkan master lain disini

/* =========================
   MAIN
========================= */

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	var err error
	for i := 1; i <= 5; i++ {
		db, err = sql.Open("postgres", dbURL)
		if err == nil && db.Ping() == nil {
			log.Println("Connected to database")
			break
		}
		log.Println("Retry DB connection in 2s...")
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	http.HandleFunc("/api/", genericHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("Server running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

/* =========================
   GENERIC HANDLER
========================= */

func genericHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse path: /api/{master}/{id}/{action}
	path := strings.TrimPrefix(r.URL.Path, "/api/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 || parts[0] == "" {
		http.Error(w, "Master table not specified", http.StatusBadRequest)
		return
	}

	masterKey := parts[0]
	config, ok := masterTables[masterKey]
	if !ok {
		http.Error(w, "Master table not found", http.StatusNotFound)
		return
	}

	// ID parsing jika ada
	id := 0
	if len(parts) > 1 && parts[1] != "" {
		var err error
		id, err = strconv.Atoi(parts[1])
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}
	}

	action := ""
	if len(parts) > 2 {
		action = parts[2]
	}

	switch r.Method {
	case http.MethodGet:
		if id > 0 {
			getByID(w, config, id)
		} else {
			getList(w, r, config)
		}
	case http.MethodPost:
		createItem(w, r, config)
	case http.MethodPut:
		if action == "restore" && id > 0 {
			restoreItem(w, config, id)
		} else if id > 0 {
			updateItem(w, r, config, id)
		} else {
			http.Error(w, "Invalid request", http.StatusBadRequest)
		}
	case http.MethodDelete:
		if id > 0 {
			deleteItem(w, config, id)
		} else {
			http.Error(w, "ID required", http.StatusBadRequest)
		}
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

/* =========================
   GENERIC CRUD FUNCTIONS
========================= */

func getByID(w http.ResponseWriter, config MasterConfig, id int) {
	query := fmt.Sprintf("SELECT %s FROM %s WHERE id=$1 AND deleted_at IS NULL", strings.Join(config.Fields, ","), config.TableName)
	row := db.QueryRow(query, id)

	values := make([]interface{}, len(config.Fields))
	valuePtrs := make([]interface{}, len(config.Fields))
	for i := range config.Fields {
		valuePtrs[i] = &values[i]
	}

	err := row.Scan(valuePtrs...)
	if err == sql.ErrNoRows {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	item := make(map[string]interface{})
	for i, f := range config.Fields {
		item[f] = values[i]
	}

	json.NewEncoder(w).Encode(item)
}

func getList(w http.ResponseWriter, r *http.Request, config MasterConfig) {
	page, limit, offset := getPagination(r)
	search := r.URL.Query().Get("q")
	searchQuery := "%" + search + "%"

	// Count total
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE deleted_at IS NULL AND (kode ILIKE $1 OR nama ILIKE $1)", config.TableName)
	db.QueryRow(countQuery, searchQuery).Scan(&total)

	totalPage := int(math.Ceil(float64(total) / float64(limit)))

	// Get rows
	query := fmt.Sprintf("SELECT %s FROM %s WHERE deleted_at IS NULL AND (kode ILIKE $1 OR nama ILIKE $1) ORDER BY id LIMIT $2 OFFSET $3", strings.Join(config.Fields, ","), config.TableName)
	rows, err := db.Query(query, searchQuery, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(config.Fields))
		valuePtrs := make([]interface{}, len(config.Fields))
		for i := range config.Fields {
			valuePtrs[i] = &values[i]
		}
		rows.Scan(valuePtrs...)
		item := make(map[string]interface{})
		for i, f := range config.Fields {
			item[f] = values[i]
		}
		results = append(results, item)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"page":       page,
		"limit":      limit,
		"total_data": total,
		"total_page": totalPage,
		"data":       results,
	})
}

func createItem(w http.ResponseWriter, r *http.Request, config MasterConfig) {
	// Decode JSON ke map[string]interface{}
	var data map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Build query dynamically
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

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING %s", config.TableName, strings.Join(fields, ","), strings.Join(placeholders, ","), strings.Join(config.Fields, ","))
	row := db.QueryRow(query, values...)

	result := make([]interface{}, len(config.Fields))
	ptrs := make([]interface{}, len(config.Fields))
	for i := range config.Fields {
		ptrs[i] = &result[i]
	}
	if err := row.Scan(ptrs...); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	item := make(map[string]interface{})
	for i, f := range config.Fields {
		item[f] = result[i]
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Created successfully",
		"data":    item,
	})
}

func updateItem(w http.ResponseWriter, r *http.Request, config MasterConfig, id int) {
	var data map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
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
	values = append(values, id)
	query := fmt.Sprintf("UPDATE %s SET %s, updated_at=NOW() WHERE id=$%d AND deleted_at IS NULL", config.TableName, strings.Join(fields, ","), i)
	result, err := db.Exec(query, values...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Updated successfully",
	})
}

func deleteItem(w http.ResponseWriter, config MasterConfig, id int) {
	query := fmt.Sprintf("UPDATE %s SET deleted_at=NOW() WHERE id=$1 AND deleted_at IS NULL", config.TableName)
	result, err := db.Exec(query, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"message": "Deleted successfully"})
}

func restoreItem(w http.ResponseWriter, config MasterConfig, id int) {
	query := fmt.Sprintf("UPDATE %s SET deleted_at=NULL WHERE id=$1", config.TableName)
	result, err := db.Exec(query, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"message": "Restored successfully"})
}

/* =========================
   HELPERS
========================= */

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
