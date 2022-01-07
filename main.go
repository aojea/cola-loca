package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/sys/unix"
)

var database string

func init() {
	flag.StringVar(&database, "database", "./cola.db", "Specify the database filename. Default ./cola.db")

}

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

func main() {
	flag.Parse()
	// trap Ctrl+C and call cancel on the context
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	// Enable signal handler
	signalCh := make(chan os.Signal, 2)
	defer func() {
		close(signalCh)
		cancel()
	}()

	signal.Notify(signalCh, os.Interrupt, unix.SIGINT)
	go func() {
		select {
		case <-signalCh:
			log.Printf("Exiting: received signal")
			cancel()
		case <-ctx.Done():
		}
	}()
	app := NewApp(database)
	app.Run(ctx)
}

type App struct {
	mu     sync.Mutex
	router *gin.Engine
	db     *sqlx.DB
}

func NewApp(dbname string) *App {
	a := &App{}
	// database
	_db, err := sqlx.Connect("sqlite3", dbname)
	if err != nil {
		panic(err)
	}
	a.db = _db
	a.db.Mapper = reflectx.NewMapperFunc("json", strings.ToLower)
	a.db.MustExec(schema)
	// API
	a.router = gin.Default()
	v1 := a.router.Group("/api/v1")
	{
		// queues
		v1.POST("/queue", a.createQueue)
		v1.GET("/queue", a.getAllQueues)
		v1.GET("/queue/:id", a.getSingleQueue)
		v1.PUT("/queue/:id", a.updateQueue)
		v1.DELETE("/queue/:id", a.deleteQueue)
		// reservations
		v1.POST("/queue/:id/reservation", a.createReservation)
		v1.GET("/queue/:id/reservation", a.getAllReservations)
		v1.GET("/queue/:id/reservation/:rsvp", a.getSingleReservation)
		v1.PUT("/queue/:id/reservation/:rsvp", a.updateReservation)
		v1.DELETE("/queue/:id/reservation/:rsvp", a.deleteReservation)
	}

	a.router.GET("/healthz", func(c *gin.Context) {
		c.String(200, "ok")
	})
	return a
}

func (a *App) Run(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		err := a.router.Run(":3000")
		if err != nil {
			log.Printf("Error stopping http server: %v", err)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
	a.db.Close()

}

// http handlers
func (a *App) createQueue(c *gin.Context) {
	var q Queue
	if err := c.BindJSON(&q); err != nil {
		return
	}
	_, err := a.db.NamedExec(`INSERT INTO queue (name) VALUES (:name)`, q)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, q)
		return
	}
	c.IndentedJSON(http.StatusCreated, q)
}

func (a *App) getAllQueues(c *gin.Context) {
	var queues []Queue
	err := a.db.Select(&queues, "SELECT * FROM queue ORDER BY id ASC")
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, queues)
		return
	}
	c.IndentedJSON(http.StatusOK, queues)
}

func (a *App) getSingleQueue(c *gin.Context) {
	id := c.Param("id")
	var q Queue
	err := a.db.Get(&q, "SELECT * FROM queue WHERE id=$1", id)
	if err != nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "reservation not found"})
		return
	}
	c.IndentedJSON(http.StatusOK, q)

}

func (a *App) updateQueue(c *gin.Context) {
	id := c.Param("id")
	var q Queue
	_, err := a.db.Exec(`UPDATE queue SET name=$1 WHERE id = $2`, q.Name, id)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, q)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": true})
}

func (a *App) deleteQueue(c *gin.Context) {
	id := c.Param("id")
	res, err := a.db.Exec("DELETE FROM queue WHERE id=$1", id)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, res)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": true})
}

func (a *App) createReservation(c *gin.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()

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
	err = a.db.Get(&pos, "SELECT COALESCE(MAX(position), 0) FROM reservation WHERE queueid=$1", id)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, err)
		return
	}
	r.Position = pos + 1
	// default group size to 1
	if r.GroupSize == 0 {
		r.GroupSize = 1
	}
	_, err = a.db.NamedExec(`INSERT INTO reservation (name, queueid, position, phone, groupSize) 
		VALUES (:name, :queueid, :position, :phone, :groupSize)`, r)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, err)
		return
	}

	c.IndentedJSON(http.StatusCreated, r)
}

func (a *App) getAllReservations(c *gin.Context) {
	id := c.Param("id")
	var reservations []Reservation
	err := a.db.Select(&reservations, "SELECT * FROM reservation WHERE queueid=$1", id)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, reservations)
		return
	}
	c.IndentedJSON(http.StatusOK, reservations)

}

func (a *App) getSingleReservation(c *gin.Context) {
	id := c.Param("id")
	rsvp := c.Param("rsvp")
	var r Reservation
	err := a.db.Get(&r, "SELECT * FROM reservation WHERE queueid=$1 AND id=$2", id, rsvp)
	if err != nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "reservation not found"})
		return
	}
	c.IndentedJSON(http.StatusOK, r)
}

func (a *App) updateReservation(c *gin.Context) {
	id := c.Param("id")
	rsvp := c.Param("rsvp")
	var r Reservation
	_, err := a.db.Exec(`UPDATE reservation SET name=$1 WHERE queueid=$2 AND id=$3`, r.Name, id, rsvp)
	if err != nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "reservation not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": true})
}

func (a *App) deleteReservation(c *gin.Context) {
	id := c.Param("id")
	rsvp := c.Param("rsvp")
	res, err := a.db.Exec("DELETE FROM reservation WHERE queueid=$1 AND id=$2", id, rsvp)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, res)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": true})
}
