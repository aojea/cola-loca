package main

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Queue struct {
	gorm.Model
	Name string `json:"name" binding:"omitempty,min=8"`
}

type Reservation struct {
	gorm.Model
	QueueID   int    `json:"queueID,omitempty"`
	Queue     Queue  `json:"queue,omitempty"`
	Position  int    `json:"position,omitempty"`
	Name      string `json:"name" binding:"required,min=8"`
	Phone     string `json:"phone" binding:"required,min=9"`
	GroupSize int    `json:"groupSize"`
}

var db *gorm.DB

// override it for tests
var database = "./cola.db"

func main() {
	_db, err := gorm.Open(sqlite.Open(database), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		panic(err)
	}
	db = _db

	if err := db.AutoMigrate(&Queue{}, &Reservation{}); err != nil {
		panic(err)
	}

	router := gin.Default()
	v1 := router.Group("/api/v1")
	{
		// queues
		v1.POST("/queue", createQueue)
		v1.GET("/queue", getAllQueues)
		v1.GET("/queue/:id", getSingleQueue)
		v1.PUT("/queue/:id", updateQueue)
		v1.DELETE("/queue/:id", deleteQueue)
		// reservations
		v1.POST("/queue/:id/reservation", createReservation)
		v1.GET("/queue/:id/reservation", getAllReservations)
		v1.GET("/queue/:id/reservation/:rsvp", getSingleReservation)
		v1.PUT("/queue/:id/reservation/:rsvp", updateReservation)
		v1.DELETE("/queue/:id/reservation/:rsvp", deleteReservation)
	}

	router.GET("/healthz", func(c *gin.Context) {
		c.String(200, "ok")
	})
	router.Run(":3000")
}

func createQueue(c *gin.Context) {
	var newQueue Queue
	if err := c.BindJSON(&newQueue); err != nil {
		return
	}
	if err := db.Create(&newQueue).Error; err != nil {
		c.IndentedJSON(http.StatusInternalServerError, newQueue)
		return
	}
	c.IndentedJSON(http.StatusCreated, newQueue)
}

func getAllQueues(c *gin.Context) {
	var queues []Queue
	if err := db.Find(&queues).Error; err != nil {
		c.IndentedJSON(http.StatusInternalServerError, queues)
		return
	}
	c.IndentedJSON(http.StatusOK, queues)
}

func getSingleQueue(c *gin.Context) {
	id := c.Param("id")
	var q Queue
	if err := db.Where("id = ?", id).First(&q).Error; err != nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "reservation not found"})
		return
	}
	c.IndentedJSON(http.StatusOK, q)

}

func updateQueue(c *gin.Context) {
	id := c.Param("id")
	var q Queue
	db.Model(&Queue{}).Where("id = ?", id).Updates(&q)
	c.JSON(http.StatusOK, gin.H{"data": true})
}

func deleteQueue(c *gin.Context) {
	id := c.Param("id")
	var q Queue
	db.Where("id = ?", id).Delete(&q)
	c.JSON(http.StatusOK, gin.H{"data": true})
}

func createReservation(c *gin.Context) {
	id := c.Param("id")
	var r Reservation
	if err := c.ShouldBindJSON(&r); err != nil {
		fmt.Println("ERROR", err)
		c.IndentedJSON(http.StatusConflict, r)
		return
	}
	i, err := strconv.Atoi(id)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, r)
		return
	}
	// obtain queue
	r.QueueID = i
	err = db.Transaction(func(tx *gorm.DB) error {
		// get the last position in the queue
		var ids []*int64
		db.Model(&Reservation{}).Where("queue_id = ?", id).Pluck("MAX(position)", &ids)
		if len(ids) != 1 {
			return fmt.Errorf("Can not obtain the last position %v", ids)
		}
		r.Position = int(*ids[0]) + 1
		return db.Create(&r).Error
	})
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, r)
		return
	}
	c.IndentedJSON(http.StatusCreated, r)
}

func getAllReservations(c *gin.Context) {
	id := c.Param("id")
	var reservations []Reservation
	if err := db.Find(&reservations).Where("queue_id = ?", id).Error; err != nil {
		c.IndentedJSON(http.StatusInternalServerError, reservations)
		return
	}
	c.IndentedJSON(http.StatusOK, reservations)

}

func getSingleReservation(c *gin.Context) {
	id := c.Param("id")
	rsvp := c.Param("rsvp")
	var r Reservation
	if err := db.Where("queue_id = ? AND id = ?", id, rsvp).First(&r).Error; err != nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "reservation not found"})
		return
	}
	c.IndentedJSON(http.StatusOK, r)
}

func updateReservation(c *gin.Context) {
	id := c.Param("id")
	rsvp := c.Param("rsvp")
	var r Reservation
	db.Model(&Reservation{}).Where("queue_id = ? AND id = ?", id, rsvp).Updates(&r)
	c.JSON(http.StatusOK, gin.H{"data": true})
}

func deleteReservation(c *gin.Context) {
	id := c.Param("id")
	rsvp := c.Param("rsvp")
	var r Reservation
	db.Where("queue_id = ? AND id = ?", id, rsvp).Delete(&r)
	c.JSON(http.StatusOK, gin.H{"data": true})
}
