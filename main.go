package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

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
	// Connect ke DB
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
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

		// 2️⃣ /?user_id=123 → SELECT * FROM user WHERE id=?
		userID := query.Get("user_id")
		if userID != "" {
			var id int
			var kode, nama, lokasi, email string
			var tanggal_lahir string // bisa pakai time.Time jika mau
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

			fmt.Fprintf(w, "ID: %d\nKode: %s\nNama: %s\nTanggal Lahir: %s\nLokasi: %s\nEmail: %s\n",
				id, kode, nama, tanggal_lahir, lokasi, email)
			return
		}

		// Default response
		fmt.Fprintln(w, "Welcome!")
	})

	// Railway menyediakan PORT
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Println("Running on port:", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// Cara pakai:

// / → tampil: Welcome!

// /?time=true → tampil current time dari DB

// /?user_id=1 → tampil data user dengan id=1
