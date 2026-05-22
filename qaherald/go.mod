module github.com/vasic-digital/herald/qaherald

go 1.25.3

require (
	github.com/google/uuid v1.6.0
	github.com/spf13/cobra v1.10.2
	github.com/vasic-digital/herald/commons v0.0.0
)

replace github.com/vasic-digital/herald/commons => ../commons
