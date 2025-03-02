package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/pgtype"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

var dsn = "host=localhost user=postgres password=123 dbname=webhook_monitoring port=5432 sslmode=disable"
var db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{NamingStrategy: schema.NamingStrategy{
	SingularTable: true,
}})

var logger *zap.Logger

type MasterWebhookServer struct {
	WebhookServerUrl string `json:"webhookServerUrl"`
}

type Producer struct {
	Id           uint      `json:"id"`
	URL          string    `json:"url"`
	LastAccessed time.Time `json:"lastAccessed"`
}

type RequestDetails struct {
	Id             uint         `json:"id"`
	ProducerId     uint         `json:"producerId"`
	URL            string       `json:"url"`
	Timestamp      time.Time    `json:"timestamp"`
	HttpMethod     string       `json:"httpMethod"`
	Headers        pgtype.JSONB `json:"headers"`
	ResponseStatus uint         `json:"responseStatus"`
	ResponseTime   float32      `json:"responseTime"`
}

func main() {
	logger, err = zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	router := gin.Default()
	router.GET("/masterWebhookServer", getMasterWebhookServer)
	router.POST("/producer", registerProducer)
	router.POST("/request", logRequest)
	router.DELETE("/purge", purgeRequestRecords)

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
		producer.LastAccessed = time.Now()
		db.Create(&producer)
		c.JSON(http.StatusCreated, producer)
	} else {
		c.JSON(http.StatusOK, existingProducer)
	}
}

func logRequest(c *gin.Context) {
	var requestDetails RequestDetails

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, "Unable to read body")
		return
	}

	var requestBody map[string]interface{}
	err = json.Unmarshal(body, &requestBody)
	if err != nil {
		panic(err)
	}

	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	if c.BindJSON(&requestDetails) != nil {
		return
	}

	requestDetails.Headers.Set(requestBody["headers"])

	result := db.Create(&requestDetails)

	if result.Error != nil {
		logger.Error(result.Error.Error())
		c.JSON(http.StatusInternalServerError, "Record Creation Failed!")
	}

	c.JSON(http.StatusCreated, requestDetails)
}

func purgeRequestRecords(c *gin.Context) {
	daysToKeep, err := strconv.Atoi(c.Query("DaysToKeep"))
	if err != nil {
		logger.Error(err.Error())
		c.JSON(http.StatusInternalServerError, false)
	}

	producerId, err := strconv.Atoi(c.Query("ProducerId"))
	if err != nil {
		logger.Error(err.Error())
		c.JSON(http.StatusInternalServerError, false)
	}

	now := time.Now()
	detailsToKeepDate := now.AddDate(0, 0, -daysToKeep)

	result := db.Delete(&RequestDetails{}, "timestamp < ? AND producer_Id = ?", detailsToKeepDate, producerId)

	if result.Error != nil {
		logger.Error(result.Error.Error())
		c.JSON(http.StatusInternalServerError, false)
	}

	c.JSON(http.StatusOK, true)
}

func ExtractHeaderJsonData(details RequestDetails) map[string]interface{} {
	var data map[string]interface{}
	if err := json.Unmarshal(details.Headers.Bytes, &data); err != nil {
		logger.Fatal("Failed to unmarshal JSONB bytes:")
	}

	if str, ok := data["Bytes"].(string); ok {
		decoded, err := base64.StdEncoding.DecodeString(str)
		if err != nil {
			fmt.Println(err)
		}

		if err := json.Unmarshal([]byte(string(decoded[:])), &data); err != nil {
			logger.Fatal("Failed to unmarshal JSONB string:")
		}
	}

	delete(data, "Bytes")
	delete(data, "Status")

	return data
}
