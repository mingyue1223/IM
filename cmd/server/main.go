// Package main is the GoIM server entry point.
// Blank imports ensure all project dependencies are tracked in go.mod
// and survive go mod tidy. They will be replaced with real usage in subsequent tasks.
package main

import (
	_ "github.com/gin-gonic/gin"
	_ "github.com/golang-jwt/jwt/v5"
	_ "github.com/gorilla/websocket"
	_ "github.com/rabbitmq/amqp091-go"
	_ "github.com/redis/go-redis/v9"
	_ "github.com/stretchr/testify/assert"
	_ "go.uber.org/zap"
	_ "golang.org/x/crypto/bcrypt"
	_ "gopkg.in/yaml.v3"
)

func main() {}
