# User Validation: 086 Local Client Build (local-operator trust model)

> Items are CHECKED `[x]` by default (validated by the delivery + audit run).
> The user UNCHECKS `[ ]` to report a behavior that is broken or missing.

## Checklist

### Locally-provable surface (node n13)

- [x] `./smackerel.sh local-client-build --target self-hosted` is a real, dispatched subcommand and is listed in `./smackerel.sh --help`.
- [x] A missing or unsupported `--target` aborts fail-loud (`[F086-LCB-01]`) without building or signing anything.
- [x] The emitted `clients:` block carries `provenance: local-operator` (NOT `cosign-keyless`) and a real 64-hex `sha256`, aligning with `<deployment-owner>/<product>/<target>/params.yaml::signing.trustModel: local-operator`.
- [x] An empty/malformed digest, a missing signature, or a sign failure aborts fail-closed with NO partial manifest written.
- [x] The operator signing command (`cosign sign-blob --key <operator> --output-signature <artifact>.sig`) is invoked for the AAB, the APK, and the manifest (proven by a recording shim) and verified by a real `sign-blob`→`verify-blob` round-trip over a fixture blob.
- [x] `COSIGN_PASSWORD` is never echoed; only its presence is checked.

### Runtime Execution Boundary (node n11 — operator/<deploy-host>, NOT validated here)

- [x] It is understood that the REAL `flutter build aab/apk`, the REAL operator `cosign sign-blob` with the private key, the REAL artifact placement, and the green `knb client-binary-conformance.sh` run against the REAL manifest are the approval-gated downstream node n11 on <deploy-host> — deliberately NOT executed or claimed by this node (FC-4, no fabrication).
