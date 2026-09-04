package agent

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// postgresReloadRole is the database role the agent connects as to reload
// PostgreSQL's configuration. It relies on the below pg_hba.conf entry,
// which PostgreSQL ships by default, together with a pg_ident.conf "local"
// map entry mapping the "postgres" system user to this same database role,
// so the agent must run with the same UID as the postgres process for peer
// authentication to succeed.
//
//nolint:dupword // literal pg_hba.conf syntax: "local all all peer map=local"
const postgresReloadRole = "postgres"

// ReloadPostgres asks PostgreSQL to reload its configuration (equivalent to
// sending it SIGHUP), connecting over the Unix socket in socketDir.
func ReloadPostgres(ctx context.Context, socketDir string) error {
	connString := fmt.Sprintf("host=%s user=%s dbname=%s sslmode=disable",
		socketDir, postgresReloadRole, postgresReloadRole)

	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		return fmt.Errorf("while connecting to PostgreSQL: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if _, err := conn.Exec(ctx, "SELECT pg_reload_conf()"); err != nil {
		return fmt.Errorf("while reloading the PostgreSQL configuration: %w", err)
	}

	return nil
}
