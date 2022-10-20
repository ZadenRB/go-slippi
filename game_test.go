package slippi

import (
	"fmt"
	"os"
	"testing"
)

func TestNewSlpGameFromBytes(t *testing.T) {

}

func TestNewSlpGameFromFile(t *testing.T) {
	f, err := os.Open("game.slp")
	if err != nil {
		t.Error(err)
		return
	}

	game, err := NewSlpGameFromFile(f, nil)
	gameInfo, err := game.GetGameInfo()
	if err != nil {
		t.Error(err)
		return
	}

	fmt.Println(gameInfo.Stage)
}
