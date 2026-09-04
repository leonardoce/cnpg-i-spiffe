# CNPG-SPIFFE Plugin

A [CNPG-I](https://github.com/cloudnative-pg/cnpg-i) plugin that gives
[CloudNativePG](https://github.com/cloudnative-pg/cloudnative-pg/) PostgreSQL
Pods access to X.509 SVIDs issued by a
[SPIFFE/SPIRE](https://spiffe.io/) deployment already running in the cluster.

For every instance Pod, the plugin:

1. Injects this plugin's own image, running its `agent` subcommand, as a
   native sidecar (an init container with `restartPolicy: Always`), which
   fetches and rotates SVIDs from the SPIRE Workload API.
2. Mounts the node's SPIRE Agent Workload API socket (hostPath) into the
   sidecar only.
3. Mounts a shared, `tmpfs`-backed volume into both the sidecar
   (read-write) and the `postgres` container (read-only), where the sidecar
   writes the fetched SVID, private key and trust bundle.
4. Mounts PostgreSQL's own Unix socket directory (reusing whatever volume
   the operator already mounted it on in the `postgres` container) into the
   sidecar, and runs the agent with the same UID/GID as the `postgres` user
   inside the image. After every SVID rotation, the agent connects over
   that socket — authenticated by `pg_hba.conf`'s default `local all all
   peer map=local` entry — and runs `SELECT pg_reload_conf();`, the same
   reload PostgreSQL performs on `SIGHUP`.
5. Serves the CNPG-i `Postgres` service from the same sidecar process, over a
   Unix socket under the `plugins` volume shared with the instance manager.
   Its `EnrichConfiguration` RPC overrides the `ssl_ca_file` GUC to point at
   the trust bundle written in step 3, so PostgreSQL verifies client
   certificates against the SPIFFE trust domain.

Wiring `ssl_cert_file`/`ssl_key_file` (PostgreSQL's own server certificate)
to the SVID written in step 3 is not done by the plugin yet — only
`ssl_ca_file` is currently overridden.

This plugin uses
the [pluginhelper](https://github.com/cloudnative-pg/cnpg-i-machinery/tree/main/pkg/pluginhelper)
from [`cnpg-i-machinery`](https://github.com/cloudnative-pg/cnpg-i-machinery) to
simplify its implementation.

## Limitations

* **Single-instance clusters only.** When CloudNativePG bootstraps a new
  replica, it invokes `pg_basebackup` against the primary with a hardcoded
  certificate path, rather than the SVID/key/bundle paths this plugin wires
  up via `EnrichConfiguration`. Until that path is made configurable (or this
  plugin hooks into the bootstrap process too), only single-instance
  `Cluster`s are supported.
* **Relies on SPIRE copying the first DNS SAN into the certificate's CN.**
  The SPIRE registration entry above passes `-dns streaming_replica`, adding
  `streaming_replica` as the SVID's first DNS SAN. With SPIRE's current
  behavior, that first DNS SAN is also copied into the certificate's CN
  field. PostgreSQL's client-certificate authentication today only checks
  the CN, not the SPIFFE ID carried in the SAN URI — so it's this incidental
  DNS-to-CN copying, not a stable, documented SPIRE guarantee, that makes
  authentication work. A future SPIRE change, or PostgreSQL gaining SAN URI
  support, would require revisiting this.

## Configuration

The plugin is configured through `parameters` on the `Cluster`'s plugin entry
(`spec.plugins[].parameters`, see the examples below). Every parameter is
optional except `sidecarImage`:

| Parameter              | Default                                     | Description                                                                                   |
|-------------------------|----------------------------------------------|-------------------------------------------------------------------------------------------------|
| `sidecarImage`          | *(required, no default)*                     | Image used for the injected sidecar. This is this plugin's own image (it runs the `agent` subcommand), so it must be set to whatever image this plugin was deployed with — there's no stable public tag to default to. |
| `spireAgentSocketPath`  | `/run/spire/agent-sockets/spire-agent.sock`  | Path, on the node, of the SPIRE Agent's Workload API socket. Its parent directory is hostPath-mounted into the sidecar. |
| `certsMountPath`        | `/spiffe-certs`                              | Where the SVID/key/bundle files are mounted in both the sidecar and the `postgres` container.   |
| `certsVolumeMedium`     | `Memory`                                     | Storage medium of the certs volume: `Memory` (tmpfs, keeps key material off disk) or `Disk`.     |
| `svidFileName`          | `svid.pem`                                   | File name used to store the X.509 SVID.                                                          |
| `svidKeyFileName`       | `svid_key.pem`                               | File name used to store the SVID's private key.                                                  |
| `svidBundleFileName`    | `svid_bundle.pem`                            | File name used to store the X.509 trust bundle.                                                  |
| `postgresSocketDir`     | `/controller/run`                            | Directory holding PostgreSQL's Unix socket, mounted into the sidecar so it can reload PostgreSQL after every SVID rotation. |

## Prerequisites

* A [SPIRE](https://spiffe.io/docs/latest/spire-about/) deployment already
  running in the cluster, with the SPIRE Agent's Workload API socket exposed
  on the nodes at (or symlinked to) `spireAgentSocketPath`. See the
  [spire-tutorials quickstart](https://github.com/spiffe/spire-tutorials/tree/main/k8s/quickstart)
  for a reference setup.
* The plugin needs permission to read `Cluster` objects cluster-wide, since
  Clusters using it can live in any namespace (see `kubernetes/rbac.yaml`).

## Running the plugin

To see the plugin in execution, you need a Kubernetes cluster running (we'll
use [Kind](https://kind.sigs.k8s.io)), the
[CloudNativePG](https://github.com/cloudnative-pg/cloudnative-pg/) operator, a
SPIRE deployment, and [cert-manager](https://cert-manager.io/) (needed by the
plugin to communicate with the operator).

### 1. Create the cluster and install CloudNativePG

``` shell
kind create cluster --name cnpg-i-spiffe
# Choose the latest version of CloudNativePG (at least 1.24)
kubectl apply --server-side -f \
  https://github.com/cloudnative-pg/cloudnative-pg/releases/download/vX.Y.Z/cnpg-X.Y.Z.yaml
```

### 2. Deploy SPIRE and register the workload

Deploy SPIRE using the
[spire-tutorials quickstart](https://github.com/spiffe/spire-tutorials/tree/main/k8s/quickstart)
(`git clone` the repo and run the commands below from its
`k8s/quickstart` directory):

``` shell
kubectl apply -k .

# Registers the node itself with the SPIRE server
sh create-node-registration-entry.sh
```

Then register a workload entry for each `Cluster` that will use the plugin,
matching the Pod's namespace and `ServiceAccount`. For the bundled
[`cluster-example`](doc/examples/cluster-example.yaml), running in the
`default` namespace with its default `ServiceAccount`:

``` shell
kubectl exec -n spire spire-server-0 -- \
  /opt/spire/bin/spire-server entry create \
  -spiffeID spiffe://example.org/ns/default/sa/cluster-example \
  -parentID spiffe://example.org/ns/spire/sa/spire-agent \
  -selector k8s:ns:default \
  -selector k8s:sa:cluster-example \
  -dns streaming_replica
```

You can verify the registration entries at any time with:

``` shell
kubectl exec -n spire spire-server-0 -- /opt/spire/bin/spire-server entry show
```

### 3. Install cert-manager

``` shell
# Choose the latest version of cert-manager
kubectl apply -f \
  https://github.com/cert-manager/cert-manager/releases/download/vX.Y.Z/cert-manager.yaml
```

(Alternatively, the [`cmctl`](https://cert-manager.io/docs/reference/cmctl/)
CLI can install cert-manager with `cmctl x install`.)

### 4. Install the plugin

If you're developing the plugin locally, build it and deploy it to the Kind
cluster in one step with:

``` shell
task local-kind-deploy
```

Otherwise, install a released version of the plugin by applying its manifest.
The easiest way to obtain the manifest may be as an artifact created by the
[`release-please` workflow](https://github.com/leonardoce/cnpg-i-spiffe/actions/workflows/release-please.yml).
You can download it and apply it locally:

``` shell
kubectl apply -f LOCAL-FOLDER/manifest.yaml
```

<!-- TODO: reevaluate on release and set release-please to automatically update it-->

### 5. Create a Cluster resource

Finally, create a cluster resource to see the plugin in action. There are two
examples in the `doc/examples` directory:

1. [Cluster with explicit parameters](doc/examples/cluster-example.yaml):
   overrides the SPIRE Agent socket path and certs volume settings used by the
   injected sidecar.
2. [Cluster with only `sidecarImage` set](doc/examples/cluster-example-no-parameters.yaml):
   lets the plugin apply its built-in defaults (SPIRE Agent socket path,
   certs volume settings, Postgres socket directory) for everything else
   when reconciling the Pod.

### 6. Verify SPIFFE-authenticated connections

The certificate the sidecar wrote to `/spiffe-certs` can authenticate a
client connection as the `streaming_replica` role, without a password,
because:

* the plugin's `EnrichConfiguration` implementation set `ssl_ca_file` to the
  SPIFFE trust bundle, so PostgreSQL verifies the client certificate against
  it;
* the SPIRE registration entry's `-dns streaming_replica` option made
  `streaming_replica` both a SAN and (per the second limitation above) the
  certificate's CN, which PostgreSQL's `cert` authentication method maps to
  the role of the same name.

Exec into the `postgres` container of `cluster-example-1` and connect with
`psql`, presenting that certificate as the client cert:

``` shell
kubectl exec -it cluster-example-1 -c postgres -- psql \
  "dbname=postgres host=cluster-example-rw user=streaming_replica sslmode=verify-full \
   sslrootcert=/controller/certificates/server-ca.crt \
   sslkey=/spiffe-certs/svid_key.pem sslcert=/spiffe-certs/svid.pem"
```

(`sslrootcert` here verifies the *server's* certificate — still the one
CNPG itself manages, since the plugin only overrides `ssl_ca_file` — and is
unrelated to the SPIFFE trust bundle used to verify the *client* certificate
above.)

The following confirms the connection was authenticated by certificate
rather than by password, and shows the client certificate's DN — with
`CN=streaming_replica`, per the second limitation above:

``` sql
SELECT usename, ssl, client_dn FROM pg_stat_ssl JOIN pg_stat_activity USING (pid)
  WHERE pid = pg_backend_pid();
```

```
psql (18.6 (Debian 18.6-1.pgdg13+2))
SSL connection (protocol: TLSv1.3, cipher: TLS_AES_256_GCM_SHA384, compression: off, ALPN: postgresql)
Type "help" for help.

postgres=> SELECT usename, ssl, client_dn FROM pg_stat_ssl JOIN pg_stat_activity USING (pid)
postgres->   WHERE pid = pg_backend_pid();
      usename      | ssl |             client_dn
-------------------+-----+------------------------------------
 streaming_replica | t   | /C=US/O=SPIRE/CN=streaming_replica
(1 row)
```