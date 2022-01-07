package main

import (
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
	_ "github.com/mattn/go-sqlite3"
)

const schema = `
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS queue (
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS reservation (
	id INTEGER PRIMARY KEY,
	queueid INTEGER,
	position INTEGER,
	name TEXT NOT NULL,
	phone TEXT NOT NULL UNIQUE,
	groupsize INTEGER,
	FOREIGN KEY (queueid) REFERENCES queue (id) ON DELETE CASCADE
);
`

type Queue struct {
	ID   int64  `json:"id"`
	Name string `json:"name" binding:"omitempty,min=8"`
}

type Reservation struct {
	ID        int64  `json:"id"`
	QueueID   int64  `json:"queueid,omitempty"`
	Queue     Queue  `json:"queue,omitempty"`
	Position  int64  `json:"position,omitempty"`
	Name      string `json:"name" binding:"required,min=8"`
	Phone     string `json:"phone" binding:"required,min=9"`
	GroupSize int64  `json:"groupsize"`
}

var db *sqlx.DB

// override it for tests
var database = "./cola.db"

var mu sync.Mutex

func main() {

	// database
	_db, err := sqlx.Connect("sqlite3", database)
	if err != nil {
		panic(err)
	}
	db = _db
	defer db.Close()

	db.Mapper = reflectx.NewMapperFunc("json", strings.ToLower)
	db.MustExec(schema)

	// API
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
	var q Queue
	if err := c.BindJSON(&q); err != nil {
		return
	}
	_, err := db.NamedExec(`INSERT INTO queue (name) VALUES (:name)`, q)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, q)
		return
	}
	c.IndentedJSON(http.StatusCreated, q)
}

func getAllQueues(c *gin.Context) {
	var queues []Queue
	err := db.Select(&queues, "SELECT * FROM queue ORDER BY id ASC")
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, queues)
		return
	}
	c.IndentedJSON(http.StatusOK, queues)
}

func getSingleQueue(c *gin.Context) {
	id := c.Param("id")
	var q Queue
	err := db.Get(&q, "SELECT * FROM queue WHERE id=$1", id)
	if err != nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "reservation not found"})
		return
	}
	c.IndentedJSON(http.StatusOK, q)

}

func updateQueue(c *gin.Context) {
	id := c.Param("id")
	var q Queue
	_, err := db.Exec(`UPDATE queue SET name=$1 WHERE id = $2`, q.Name, id)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, q)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": true})
}

func deleteQueue(c *gin.Context) {
	id := c.Param("id")
	res, err := db.Exec("DELETE FROM queue WHERE id=$1", id)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, res)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": true})
}

func createReservation(c *gin.Context) {
	mu.Lock()
	defer mu.Unlock()

	id := c.Param("id")
	var r Reservation
	if err := c.ShouldBindJSON(&r); err != nil {
		c.IndentedJSON(http.StatusConflict, r)
		return
	}
	i, err := strconv.Atoi(id)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, r)
		return
	}
	// obtain queue
	r.QueueID = int64(i)
	// get the last position in the queue
	var pos int64
	err = db.Get(&pos, "SELECT COALESCE(MAX(position), 0) FROM reservation WHERE queueid=$1", id)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, err)
		return
	}
	r.Position = pos + 1
	// default group size to 1
	if r.GroupSize == 0 {
		r.GroupSize = 1
	}
	_, err = db.NamedExec(`INSERT INTO reservation (name, queueid, position, phone, groupSize) 
		VALUES (:name, :queueid, :position, :phone, :groupSize)`, r)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, err)
		return
	}

	c.IndentedJSON(http.StatusCreated, r)
}

func getAllReservations(c *gin.Context) {
	id := c.Param("id")
	var reservations []Reservation
	err := db.Select(&reservations, "SELECT * FROM reservation WHERE queueid=$1", id)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, reservations)
		return
	}
	c.IndentedJSON(http.StatusOK, reservations)

}

func getSingleReservation(c *gin.Context) {
	id := c.Param("id")
	rsvp := c.Param("rsvp")
	var r Reservation
	err := db.Get(&r, "SELECT * FROM reservation WHERE queueid=$1 AND id=$2", id, rsvp)
	if err != nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "reservation not found"})
		return
	}
	c.IndentedJSON(http.StatusOK, r)
}

func updateReservation(c *gin.Context) {
	id := c.Param("id")
	rsvp := c.Param("rsvp")
	var r Reservation
	_, err := db.Exec(`UPDATE reservation SET name=$1 WHERE queueid=$2 AND id=$3`, r.Name, id, rsvp)
	if err != nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "reservation not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": true})
}

func deleteReservation(c *gin.Context) {
	id := c.Param("id")
	rsvp := c.Param("rsvp")
	res, err := db.Exec("DELETE FROM reservation WHERE queueid=$1 AND id=$2", id, rsvp)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, res)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": true})
}
