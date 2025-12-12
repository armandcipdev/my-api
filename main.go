package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"

	_ "github.com/lib/pq"
)

func main() {
	// Ambil DATABASE_URL dari Railway
	// dbURL := os.Getenv("DATABASE_URL")
	// if dbURL == "" {
	// 	log.Fatal("DATABASE_URL not set")
	// }
	// dbURL := "postgresql://postgres:password@postgres.railway.internal:5432/railway"
	dbURL := "postgresql://postgres:tiJDUvMQYrRYHkgCYKoMYrFWbAGNMjRi@interchange.proxy.rlwy.net:11133/railway"

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("DB connection error: ", err)
	}
	defer db.Close()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		// 1️⃣ /?time=true → SELECT NOW()
		if query.Get("time") == "true" {
			var now string
			err := db.QueryRow("SELECT NOW()").Scan(&now)
			if err != nil {
				http.Error(w, "DB error: "+err.Error(), 500)
				return
			}
			fmt.Fprintf(w, "Current time from DB: %s\n", now)
			return
		}

		// 2️⃣ /?user_id=123 → SELECT * FROM user_pengguna WHERE id=?
		userID := query.Get("user_id")
		if userID != "" {
			var id int
			var kode, nama, tanggal_lahir, lokasi, email string
			err := db.QueryRow(
				`SELECT id, kode, nama, tanggal_lahir, lokasi, email 
				 FROM user_pengguna WHERE id=$1`, userID).Scan(&id, &kode, &nama, &tanggal_lahir, &lokasi, &email)
			if err != nil {
				if err == sql.ErrNoRows {
					http.Error(w, "User not found", 404)
					return
				}
				http.Error(w, "DB error: "+err.Error(), 500)
				return
			}

			fmt.Fprintf(w,
				"ID: %d\nKode: %s\nNama: %s\nTanggal Lahir: %s\nLokasi: %s\nEmail: %s\n",
				id, kode, nama, tanggal_lahir, lokasi, email)
			return
		}

		// 3️⃣ /?user=list → List semua user_pengguna
		if query.Get("user") == "list" {
			rows, err := db.Query(`SELECT id, kode, nama, tanggal_lahir, lokasi, email FROM user_pengguna`)
			if err != nil {
				http.Error(w, "DB error: "+err.Error(), 500)
				return
			}
			defer rows.Close()

			// Print header kolom
			fmt.Fprintf(w, "ID | Kode | Nama | Tanggal Lahir | Lokasi | Email\n")
			fmt.Fprintf(w, "-----------------------------------------------\n")

			// Print data tiap row
			for rows.Next() {
				var id int
				var kode, nama, tanggal_lahir, lokasi, email string
				err := rows.Scan(&id, &kode, &nama, &tanggal_lahir, &lokasi, &email)
				if err != nil {
					http.Error(w, "Row scan error: "+err.Error(), 500)
					return
				}
				fmt.Fprintf(w, "%d | %s | %s | %s | %s | %s\n", id, kode, nama, tanggal_lahir, lokasi, email)
			}

			if err := rows.Err(); err != nil {
				http.Error(w, "Rows error: "+err.Error(), 500)
				return
			}

			return
		}

		// Default response
		fmt.Fprintln(w, "Welcome!")
	})

	// Railway akan memberikan PORT via env
	port := "8080"
	if p := getenv("PORT"); p != "" {
		port = p
	}

	fmt.Println("Running on port:", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// helper function supaya tidak error kalau getenv kosong
func getenv(key string) string {
	if v := key; v != "" {
		return v
	}
	return ""
}

// Cara pakai:

// / → tampil: Welcome!
// /?time=true → tampil current time dari DB
// /?user_id=1 → tampil data user dengan id=1
// /?user=list → tampil list semua user_pengguna
