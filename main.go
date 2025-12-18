package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

/* =========================
   STRUCT
========================= */

type User struct {
	ID           int       `json:"id"`
	Kode         string    `json:"kode"`
	Nama         string    `json:"nama"`
	TanggalLahir string    `json:"tanggal_lahir"`
	Lokasi       string    `json:"lokasi"`
	Email        string    `json:"email"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

/* =========================
   GLOBAL DB
========================= */

var db *sql.DB

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

	http.HandleFunc("/users", usersHandler)
	http.HandleFunc("/users/", userHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Println("Server running on port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

/* =========================
   HANDLERS
========================= */

func usersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {

	// ================= CREATE =================
	case http.MethodPost:
		var u User
		if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if err := validateUser(u); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		err := db.QueryRow(
			`INSERT INTO user_pengguna
			 (kode, nama, tanggal_lahir, lokasi, email)
			 VALUES ($1,$2,$3,$4,$5)
			 RETURNING id, created_at, updated_at`,
			u.Kode, u.Nama, u.TanggalLahir, u.Lokasi, u.Email,
		).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(u)

	// ================= READ ALL =================
	case http.MethodGet:
		page, limit, offset := getPagination(r)
		search := r.URL.Query().Get("q")
		searchQuery := "%" + search + "%"

		var total int
		err := db.QueryRow(
			`SELECT COUNT(*)
			 FROM user_pengguna
			 WHERE deleted_at IS NULL
			 AND (
				kode ILIKE $1 OR
				nama ILIKE $1 OR
				email ILIKE $1
			 )`,
			searchQuery,
		).Scan(&total)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		totalPage := int(math.Ceil(float64(total) / float64(limit)))

		rows, err := db.Query(
			`SELECT id, kode, nama, tanggal_lahir, lokasi, email,
			        created_at, updated_at
			 FROM user_pengguna
			 WHERE deleted_at IS NULL
			 AND (
				kode ILIKE $1 OR
				nama ILIKE $1 OR
				email ILIKE $1
			 )
			 ORDER BY id
			 LIMIT $2 OFFSET $3`,
			searchQuery, limit, offset,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var users []User
		for rows.Next() {
			var u User
			rows.Scan(
				&u.ID,
				&u.Kode,
				&u.Nama,
				&u.TanggalLahir,
				&u.Lokasi,
				&u.Email,
				&u.CreatedAt,
				&u.UpdatedAt,
			)
			users = append(users, u)
		}

		response := map[string]interface{}{
			"page":       page,
			"limit":      limit,
			"total_data": total,
			"total_page": totalPage,
			"data":       users,
		}

		json.NewEncoder(w).Encode(response)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func userHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	path := strings.TrimPrefix(r.URL.Path, "/users/")
	parts := strings.Split(path, "/")

	id, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	// ================= RESTORE =================
	if len(parts) == 2 && parts[1] == "restore" {
		if r.Method != http.MethodPut {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		_, err := db.Exec(
			`UPDATE user_pengguna
			 SET deleted_at = NULL
			 WHERE id=$1`,
			id,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{
			"message": "user restored",
		})
		return
	}

	switch r.Method {

	// ================= READ BY ID =================
	case http.MethodGet:
		var u User
		err := db.QueryRow(
			`SELECT id, kode, nama, tanggal_lahir, lokasi, email,
			        created_at, updated_at
			 FROM user_pengguna
			 WHERE id=$1 AND deleted_at IS NULL`,
			id,
		).Scan(
			&u.ID,
			&u.Kode,
			&u.Nama,
			&u.TanggalLahir,
			&u.Lokasi,
			&u.Email,
			&u.CreatedAt,
			&u.UpdatedAt,
		)

		if err == sql.ErrNoRows {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(u)

	// ================= UPDATE =================
	case http.MethodPut:
		var u User
		if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if err := validateUser(u); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		result, err := db.Exec(
			`UPDATE user_pengguna
			 SET kode=$1, nama=$2, tanggal_lahir=$3,
			     lokasi=$4, email=$5
			 WHERE id=$6 AND deleted_at IS NULL`,
			u.Kode, u.Nama, u.TanggalLahir, u.Lokasi, u.Email, id,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		affected, _ := result.RowsAffected()
		if affected == 0 {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		u.ID = id
		json.NewEncoder(w).Encode(u)

	// ================= SOFT DELETE =================
	case http.MethodDelete:
		result, err := db.Exec(
			`UPDATE user_pengguna
			 SET deleted_at = NOW()
			 WHERE id=$1 AND deleted_at IS NULL`,
			id,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		affected, _ := result.RowsAffected()
		if affected == 0 {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

/* =========================
   HELPERS
========================= */

func getPagination(r *http.Request) (page, limit, offset int) {
	page = 1
	limit = 10

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

func validateUser(u User) error {
	if strings.TrimSpace(u.Kode) == "" {
		return fmt.Errorf("kode is required")
	}
	if len(strings.TrimSpace(u.Nama)) < 3 {
		return fmt.Errorf("nama must be at least 3 characters")
	}
	if !isValidEmail(u.Email) {
		return fmt.Errorf("invalid email")
	}
	if _, err := time.Parse("2006-01-02", u.TanggalLahir); err != nil {
		return fmt.Errorf("tanggal_lahir must be YYYY-MM-DD")
	}
	return nil
}

func isValidEmail(email string) bool {
	re := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	return re.MatchString(email)
}

// 🧪 Endpoint Final
// Method	Endpoint
// POST	/users
// GET	/users?page=1&limit=10&q=andi
// GET	/users/{id}
// PUT	/users/{id}
// DELETE	/users/{id} (soft)
// PUT	/users/{id}/restore
