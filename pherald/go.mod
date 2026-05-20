module github.com/vasic-digital/herald/pherald

go 1.22

require (
	github.com/spf13/cobra v1.8.0
	github.com/vasic-digital/herald/commons v0.0.0
	github.com/vasic-digital/herald/commons_messaging v0.0.0
	github.com/vasic-digital/herald/commons_prefix v0.0.0
	github.com/vasic-digital/herald/commons_storage v0.0.0
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
)

replace (
	github.com/vasic-digital/herald/commons => ../commons
	github.com/vasic-digital/herald/commons_messaging => ../commons_messaging
	github.com/vasic-digital/herald/commons_prefix => ../commons_prefix
	github.com/vasic-digital/herald/commons_storage => ../commons_storage
)
