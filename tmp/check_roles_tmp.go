package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	db := os.Getenv("DATABASE_URL")
	pool, err := pgxpool.New(context.Background(), db)
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	rows, err := pool.Query(context.Background(), `
		SELECT u.email,
		       COALESCE(array_agg(r.name ORDER BY r.name) FILTER (WHERE r.name IS NOT NULL), ARRAY[]::text[])
		FROM RAC_users u
		LEFT JOIN RAC_user_roles ur ON ur.user_id = u.id
		LEFT JOIN RAC_roles r ON r.id = ur.role_id
		GROUP BY u.id, u.email
		ORDER BY u.email
	`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		var email string
		var roles []string
		if err := rows.Scan(&email, &roles); err != nil {
			panic(err)
		}
		fmt.Printf("%s => %v\n", email, roles)
	}

	if err := rows.Err(); err != nil {
		panic(err)
	}
}
package tmp
package main

import (










































}	}		panic(err)	if err := rows.Err(); err != nil {	}		fmt.Printf("%s => %v\n", email, roles)		}			panic(err)		if err := rows.Scan(&email, &roles); err != nil {		var roles []string		var email string	for rows.Next() {	defer rows.Close()	}		panic(err)	if err != nil {	`)		ORDER BY u.email		GROUP BY u.id, u.email		LEFT JOIN RAC_roles r ON r.id = ur.role_id		LEFT JOIN RAC_user_roles ur ON ur.user_id = u.id		FROM RAC_users u		       COALESCE(array_agg(r.name ORDER BY r.name) FILTER (WHERE r.name IS NOT NULL), ARRAY[]::text[])		SELECT u.email,	rows, err := pool.Query(context.Background(), `	defer pool.Close()	}		panic(err)	if err != nil {	pool, err := pgxpool.New(context.Background(), db)	db := os.Getenv("DATABASE_URL")func main() {)	"github.com/jackc/pgx/v5/pgxpool"	"os"	"fmt"	"context"