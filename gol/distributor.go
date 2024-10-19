package gol

import (
	"flag"
	"fmt"
	"net/rpc"
	"strconv"
	"sync"
	"time"
	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

const (
	alive = 255
	dead  = 0
)

func makeWorld(height, width int) [][]uint8 {
	world := make([][]uint8, height)
	for i := range world {
		world[i] = make([]uint8, width)
	}
	return world
}

func writeImage(p Params, c distributorChannels, turn int, world [][]uint8) {
	c.ioCommand <- ioOutput
	filename := strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(turn)
	c.ioFilename <- filename
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			if world[y][x] == alive {
				c.ioOutput <- alive
			} else {
				c.ioOutput <- dead
			}
		}
	}
	c.events <- ImageOutputComplete{turn, filename}
}

func readImage(p Params, c distributorChannels, world [][]uint8) [][]uint8 {
	c.ioCommand <- ioInput
	c.ioFilename <- strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight)
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			image := <-c.ioInput
			world[y][x] = image
			if image == alive {
				c.events <- CellFlipped{0, util.Cell{X: x, Y: y}}
			}
		}
	}
	return world
}

func findAliveCells(height, width int, world [][]uint8) []util.Cell {
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

func makeCall(client *rpc.Client, turn, imageHeight, imageWidth int, world [][]uint8) [][]uint8 {
	request := stubs.Request{Threads: turn, ImageHeight: imageHeight, ImageWidth: imageWidth, World: world}
	response := new(stubs.Response)
	client.Call(stubs.GameOfLife, request, response)
	return response.World
}

func countCall(client *rpc.Client, currentTurn, imageHeight, imageWidth int) (turn, calculateCells int) {
	request := stubs.CountCellsRequest{AllTurns: currentTurn, ImageHeight: imageHeight, ImageWidth: imageWidth}
	response := new(stubs.CountCellResponse)
	client.Call(stubs.GetAliveCells, request, response)
	return response.Turn, response.AliveCells
}

func saveCall(p Params, c distributorChannels, client *rpc.Client) {
	turn, world := turnCall(client)
	writeImage(p, c, turn, world)
}

func stateCall(client *rpc.Client, c distributorChannels, newState State) {
	turn, _ := turnCall(client)
	c.events <- StateChange{turn, newState}
}

func turnCall(client *rpc.Client) (int, [][]uint8) {
	turnResponse := new(stubs.CountCellResponse)
	err := client.Call(stubs.GetAliveCells, stubs.EmptyRequest{}, turnResponse)
	if err != nil {
		fmt.Println(err)
	}
	return turnResponse.Turn, turnResponse.TurnWorld
}

func pauseCall(client *rpc.Client, req stubs.PauseRequest) {
	err := client.Call(stubs.Pause, req, &stubs.EmptyResponse{})
	if err != nil {
		fmt.Println(err)
	}
}

var server = flag.String("server", "127.0.0.1:8030", "IP:port string to connect to as server")

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels, keyPresses <-chan rune) {

	// TODO: Create a 2D slice to store the world.
	initialWorld := makeWorld(p.ImageHeight, p.ImageWidth)

	turn := p.Turns
	readImage(p, c, initialWorld)
	flag.Parse()
	client, _ := rpc.Dial("tcp", *server)
	defer client.Close()
	ticker := time.NewTicker(2 * time.Second)
	mutex := new(sync.Mutex)
	done := make(chan bool)

	go func() {
		for {
			select {
			case key := <-keyPresses:
				if key == 'q' {
					err := client.Call(stubs.Reset, stubs.EmptyRequest{}, &stubs.EmptyResponse{})
					if err != nil {
						fmt.Println(err.Error())
					}
					if key == 's' {
						saveCall(p, c, client)
					}
					if key == 'k' {
						saveCall(p, c, client)
						stateCall(client, c, Quitting)
						err := client.Call(stubs.TurnOff, stubs.EmptyRequest{}, &stubs.EmptyResponse{})
						if err != nil {
							fmt.Println(err.Error())
						}

					}
				}
				if key == 'p' {
					pauseCall(client, stubs.PauseRequest{Command: "PAUSE"})
					stateCall(client, c, Paused)
					for {
						await := <-keyPresses
						if await == 'p' {
							pauseCall(client, stubs.PauseRequest{Command: "RESTART"})
							stateCall(client, c, Executing)
							break
						}
					}
				}
			case <-ticker.C:
				mutex.Lock()
				currentTurn, countCells := countCall(client, turn, p.ImageHeight, p.ImageWidth)
				c.events <- AliveCellsCount{currentTurn, countCells}
				mutex.Unlock()
			case <-done:
				return
			default:
			}
		}
	}()

	var receiveWorld [][]uint8

	if p.Turns == 0 {
		receiveWorld = initialWorld
	} else {
		receiveWorld = makeCall(client, turn, p.ImageHeight, p.ImageWidth, initialWorld)
	}
	done <- true

	writeImage(p, c, p.Turns, receiveWorld)

	// TODO: Report the final state using FinalTurnCompleteEvent.
	aliveCells := findAliveCells(p.ImageHeight, p.ImageWidth, receiveWorld)
	c.events <- FinalTurnComplete{p.Turns, aliveCells}

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{p.Turns, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}
