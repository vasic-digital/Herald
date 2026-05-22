// Module commons_tls is Herald's TLS cert-sourcing helper. Owns the
// dev self-signed cert auto-generation flow + cert resolution policy
// (flag > env > dev-autogen, with prod-mode fail-loud override).
//
// Catalogue-Check (§11.4.74): no-match → vendor as Herald-internal package.
// Evidence: docs/catalogue-checks/HRD-100-commons-tls.md.
//
// Licensed under the terms in ../LICENSE.
module github.com/vasic-digital/herald/commons_tls

go 1.25.3
