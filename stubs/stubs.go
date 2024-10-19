package stubs

import "uk.ac.bris.cs/gameoflife/util"

var GameOfLife = "ServerOperations.ProcessTurns"
var GetAliveCells = "ServerOperations.GetAliveCells"
var Pause = "ServerOperations.Pause"
var Reset = "ServerOperations.Reset"
var TurnOff = "ServerOperations.TurnOff"

type Request struct {
	Turns          int
	Threads        int
	ImageHeight    int
	ImageWidth     int
	CalculateCells int
	AliveCells     []util.Cell
	World          [][]uint8
}

type CountCellsRequest struct {
	AllTurns    int
	ImageHeight int
	ImageWidth  int
}

type PauseRequest struct {
	Command string
}

type EmptyRequest struct {
}

type Response struct {
	Turn           int
	CalculateCells int
	AliveCells     []util.Cell
	World          [][]uint8
}

type CountCellResponse struct {
	Turn       int
	AliveCells int
	TurnWorld  [][]uint8
}

type EmptyResponse struct {
}
