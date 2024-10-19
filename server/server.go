package main

import (
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"sync"
	"time"
	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

type ServerOperations struct{}

var (
	process    bool
	findTurn   int
	findWorld  [][]uint8
	mutex      = new(sync.Mutex)
	aliveCells []util.Cell
	pause      = make(chan int)
	restart    = make(chan int)
	turnOff    = make(chan int)
)

func makeWorld(height, width int) [][]uint8 {
	world := make([][]uint8, height)
	for i := range world {
		world[i] = make([]uint8, width)
	}
	return world
}

const (
	alive = 255
	dead  = 0
)

func unit(i, j int) int {
	return (i + j) % j
}
func calculateNeighbours(height, width, x, y int, world [][]uint8) int {
	neighbours := 0
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if !(i == 0 && j == 0) {
				h := unit(y+i, height)
				w := unit(x+j, width)
				if world[h][w] == alive {
					neighbours++
				}
			}
		}
	}
	return neighbours
}

func calculateNextState(height, width int, initialWorld [][]uint8) [][]uint8 {
	nextState := makeWorld(height, width)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			neighbours := calculateNeighbours(height, width, x, y, initialWorld)
			if initialWorld[y][x] == alive {
				if neighbours == 2 || neighbours == 3 {
					nextState[y][x] = alive
				} else {
					nextState[y][x] = dead
				}
			} else {
				if neighbours == 3 {
					nextState[y][x] = alive
				} else {
					nextState[y][x] = dead
				}
			}
		}
	}
	return nextState
}

func calculateAliveCells(height, width int, world [][]uint8) []util.Cell {
	var aliveCells []util.Cell
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if world[y][x] == alive {
				aliveCells = append(aliveCells, util.Cell{X: x, Y: y})
			}
		}
	}
	return aliveCells
}

func (s *ServerOperations) ProcessTurns(req stubs.Request, res *stubs.Response) (err error) {
	mutex.Lock()
	findWorld = req.World
	aliveCells = req.AliveCells
	findTurn = req.Turns
	mutex.Unlock()
	for findTurn < req.Threads {
		mutex.Lock()
		findWorld = calculateNextState(req.ImageHeight, req.ImageWidth, findWorld)
		aliveCells = calculateAliveCells(req.ImageHeight, req.ImageWidth, findWorld)
		findTurn++
		mutex.Unlock()
	}
	mutex.Lock()
	res.World = findWorld

	res.AliveCells = aliveCells
	res.Turn = findTurn
	mutex.Unlock()
	return
}

func (s *ServerOperations) GetAliveCells(req stubs.CountCellsRequest, res *stubs.CountCellResponse) (err error) {
	mutex.Lock()
	res.Turn = findTurn
	res.AliveCells = len(calculateAliveCells(len(findWorld), len(findWorld[0]), findWorld))
	mutex.Unlock()
	return
}

func (s *ServerOperations) Reset(req stubs.EmptyRequest, res *stubs.EmptyResponse) (err error) {
	process = false
	return
}

func (s *ServerOperations) Pause(req stubs.PauseRequest, res *stubs.EmptyResponse) (err error) {
	if req.Command == "PAUSE" {
		pause <- 1
	}
	if req.Command == "RESTART" {
		restart <- 1
	}
	return
}

func (s *ServerOperations) TurnOff(req stubs.EmptyRequest, res *stubs.EmptyResponse) (err error) {
	turnOff <- 1
	process = false
	<-time.After(1 * time.Second)
	os.Exit(0)
	return
}

func main() {
	pAddr := flag.String("port", "8030", "Port to listen on")
	flag.Parse()
	rpc.Register(&ServerOperations{})
	listener, _ := net.Listen("tcp", ":"+*pAddr)

	defer func(listener net.Listener) {
		err := listener.Close()
		if err != nil {
			fmt.Println("Error in listerner")
		}
	}(listener)

	rpc.Accept(listener)

}
