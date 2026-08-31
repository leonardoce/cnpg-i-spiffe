/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

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
