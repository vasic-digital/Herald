// Module commons_infra is Herald's L1 on-demand-infra layer per spec V3 §44
// and Universal Constitution §11.4.76.
//
// It wraps `digital.vasic.containers/pkg/compose` so Foundation tests +
// the `pherald doctor` subcommand can bring Postgres + Redis + OTel up
// on-demand from the Herald quickstart compose file, satisfying the
// on-demand-infra invariant.
//
// Licensed under the terms in ../LICENSE.
module github.com/vasic-digital/herald/commons_infra

go 1.25.0

require digital.vasic.containers v0.0.0

replace digital.vasic.containers => ../containers
