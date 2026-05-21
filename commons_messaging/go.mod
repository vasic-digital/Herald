module github.com/vasic-digital/herald/commons_messaging

go 1.22

require (
	github.com/google/uuid v1.6.0
	github.com/vasic-digital/herald/commons v0.0.0
	gopkg.in/telebot.v3 v3.3.8
)

replace github.com/vasic-digital/herald/commons => ../commons

replace gopkg.in/telebot.v3 => ../submodules/telebot
