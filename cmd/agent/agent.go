package agent

import (
	"github.com/spf13/cobra"

	agentImpl "github.com/leonardoce/cnpg-i-spiffe/internal/agent"
)

// NewCmd creates the `agent` command
func NewCmd() *cobra.Command {
	opts := agentImpl.Options{}

	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Fetch and rotate SPIFFE SVIDs, reloading PostgreSQL on every rotation",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return agentImpl.Run(cmd.Context(), opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.SpireAgentSocketPath, "spire-agent-socket-path", "/run/spire/agent-sockets/spire-agent.sock",
		"Path of the SPIRE Agent's Workload API socket")
	flags.StringVar(&opts.CertsDir, "certs-dir", "/spiffe-certs",
		"Directory where the SVID, private key and trust bundle are written")
	flags.StringVar(&opts.SVIDFileName, "svid-file-name", "svid.pem",
		"File name used to write the X.509 SVID")
	flags.StringVar(&opts.SVIDKeyFileName, "svid-key-file-name", "svid_key.pem",
		"File name used to write the SVID's private key")
	flags.StringVar(&opts.SVIDBundleFileName, "svid-bundle-file-name", "svid_bundle.pem",
		"File name used to write the X.509 trust bundle")
	flags.StringVar(&opts.PostgresSocketDir, "postgres-socket-dir", "/controller/run",
		"Directory holding PostgreSQL's Unix socket, used to reload its configuration after every SVID rotation")
	flags.StringVar(&opts.PluginPath, "plugin-path", "/plugins",
		"Directory holding the Unix socket the CNPG-i Postgres service is served on, shared with the instance manager")
	flags.StringVar(&opts.HealthCheckAddr, "health-check-address", ":8081",
		"Address the health check HTTP server listens on")
	flags.StringVar(&opts.HealthCheckLivenessPath, "health-check-liveness-path", "/live",
		"Path served by the liveness health check")
	flags.StringVar(&opts.HealthCheckReadinessPath, "health-check-readiness-path", "/ready",
		"Path served by the readiness health check")

	return cmd
}
