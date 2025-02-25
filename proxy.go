package main

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

var dsn = "host=localhost user=postgres password=123 dbname=webhook_monitoring port=5432 sslmode=disable"
var db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{NamingStrategy: schema.NamingStrategy{
	SingularTable: true,
}})

type MasterWebhookServer struct {
	WebhookServerUrl string `json:"webhookServerUrl"`
}

type Producer struct {
	Id  uint   `json:"id"`
	URL string `json:"url"`
}

func main() {
	router := gin.Default()
	router.GET("/masterWebhookServer", getMasterWebhookServer)
	router.POST("/producer", registerProducer)

	router.Run("localhost:8080")
}

func getMasterWebhookServer(c *gin.Context) {
	var masterServer MasterWebhookServer
	db.First(&masterServer)

	c.JSON(http.StatusOK, masterServer)
}

func registerProducer(c *gin.Context) {
	var producer Producer

	if c.BindJSON(&producer) != nil {
		return
	}

	var existingProducer Producer
	result := db.First(&existingProducer, "lower(url) = lower(?)", producer.URL)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		db.Create(&producer)
		c.JSON(http.StatusCreated, producer)
	} else {
		c.JSON(http.StatusOK, existingProducer)
	}
}
