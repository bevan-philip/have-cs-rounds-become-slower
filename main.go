package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	dem "github.com/markus-wa/demoinfocs-golang/v3/pkg/demoinfocs"
	common "github.com/markus-wa/demoinfocs-golang/v3/pkg/demoinfocs/common"
	events "github.com/markus-wa/demoinfocs-golang/v3/pkg/demoinfocs/events"

	_ "github.com/mattn/go-sqlite3"
)

type Round struct {
	gameId                  int64
	round                   int
	duration                int
	losingTeamName          string
	losingSide              int
	startTick               int
	endTick                 int
	endOfficialTick         int
	survivingPlayers        []uint64
	losingTeamLeftoverMoney int
	equipmentSavedValue     int
	killTicks               []int
	smokeTicks              []int
	molotovTicks            []int
	heTicks                 []int
	longestKillWait         int
	lastKillToEnd           int
	heDamage                int
}

type Player struct {
	name      string
	SteamID64 uint64
}

type Game struct {
	TeamA        string
	TeamAPlayers []uint64
	TeamB        string
	TeamBPlayers []uint64
	date         time.Time
	de_map       string
	tickrate     int
}

func main() {
	const db_file string = "csgo.db"
	// FYI - database/sql is thread-safe.
	// But we'll encounter lock contention issues (SQLite only supports 1 writer).
	// Making the database the bottleneck here.
	// Nothing to do other than switch DBMS, unforunately.
	db, err := sql.Open("sqlite3", db_file)
	if err != nil {
		log.Panic(err)
	}

	fileErr := filepath.Walk("../demos",
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if strings.HasSuffix(path, ".dem") {
				fmt.Println("Trying to parse: ", path)
				parseDemo(path, db)
			}
			return nil
		})

	if fileErr != nil {
		log.Println(fileErr)
	}
}

func parseDemo(filename string, db *sql.DB) {
	var game Game

	info, err := os.Stat(filename)
	if err != nil {
		log.Panic("failed to get file info: ", err)
	}
	game.date = info.ModTime()

	f, err := os.Open(filename)
	if err != nil {
		log.Panic("failed to open demo file: ", err)
	}
	defer f.Close()
	p := dem.NewParser(f)
	defer p.Close()

	var round Round
	var winningTeam common.Team
	var gameId int64

	p.RegisterEventHandler(func(e events.Kill) {
		round.killTicks = append(round.killTicks, p.GameState().IngameTick())
	})

	p.RegisterEventHandler(func(e events.SmokeStart) {
		round.smokeTicks = append(round.smokeTicks, p.GameState().IngameTick())
	})

	p.RegisterEventHandler(func(e events.InfernoStart) {
		round.molotovTicks = append(round.molotovTicks, p.GameState().IngameTick())
	})

	p.RegisterEventHandler(func(e events.HeExplode) {
		round.heTicks = append(round.heTicks, p.GameState().IngameTick())
	})

	p.RegisterEventHandler(func(e events.RoundStart) {
		round = Round{}
		startParse(p.GameState(), &round, &game, db, &gameId, p.Header().MapName, int(p.TickRate()))

	})

	p.RegisterEventHandler(func(e events.GamePhaseChanged) {
		if e.NewGamePhase == common.GamePhaseGameEnded {
			endParse(p.GameState(), &round, winningTeam, db)
		} else if e.NewGamePhase == common.GamePhaseStartGamePhase {
			startParse(p.GameState(), &round, &game, db, &gameId, p.Header().MapName, int(p.TickRate()))
		}
	})

	p.RegisterEventHandler(func(e events.RoundFreezetimeEnd) {
		round.startTick = p.GameState().IngameTick()
	})

	p.RegisterEventHandler(func(e events.RoundEnd) {
		if e.WinnerState != nil {
			gs := p.GameState()

			round.endTick = gs.IngameTick()
			round.duration = (round.endTick - round.startTick) / int(p.TickRate())

			winningTeam = e.Winner

			round.losingSide = int(e.WinnerState.Opponent.ID())
			round.losingTeamName = e.WinnerState.Opponent.ClanName()
		}
	})

	p.RegisterEventHandler(func(e events.RoundEndOfficial) {
		endParse(p.GameState(), &round, winningTeam, db)
	})

	p.RegisterEventHandler(func(e events.PlayerHurt) {
		if e.Weapon.Type == common.EqHE {
			round.heDamage += e.HealthDamageTaken
		}
	})

	// Parse to end
	err = p.ParseToEnd()
	if err != nil {
		log.Panic("failed to parse demo: ", err)
	}
}

func startParse(gs dem.GameState, round *Round, game *Game, db *sql.DB, gameId *int64, de_map string, tickrate int) {
	if len(gs.TeamCounterTerrorists().Members()) == 5 && len(gs.TeamTerrorists().Members()) == 5 {
		return
	}
	round.round = gs.TotalRoundsPlayed()

	if len(game.TeamAPlayers) == 0 {
		for _, s := range gs.TeamCounterTerrorists().Members() {
			game.TeamAPlayers = append(game.TeamAPlayers, s.SteamID64)
			addPlayer(db, Player{s.Name, s.SteamID64})
		}
		for _, s := range gs.TeamTerrorists().Members() {
			game.TeamBPlayers = append(game.TeamBPlayers, s.SteamID64)
			addPlayer(db, Player{s.Name, s.SteamID64})
		}
		game.TeamA = gs.TeamCounterTerrorists().ClanName()
		game.TeamB = gs.TeamTerrorists().ClanName()

		game.de_map = de_map
		if tickrate != -1 {
			game.tickrate = tickrate
		}

		TeamAPlayersJson, _ := json.Marshal(game.TeamAPlayers)
		TeamBPlayersJson, _ := json.Marshal(game.TeamBPlayers)

		res, err := db.Exec("INSERT INTO game VALUES(NULL, ?, ?, ?, ?, ?, ?, ?)", game.date.Format(time.RFC3339), game.TeamA, game.TeamB, string(TeamAPlayersJson), string(TeamBPlayersJson), game.de_map, game.tickrate)
		if err != nil {
			log.Fatal(err)
		}

		if *gameId, err = res.LastInsertId(); err != nil {
			log.Fatal(err)
		}
	}
	round.gameId = *gameId
}

func addPlayer(db *sql.DB, player Player) {
	_, err := db.Exec("INSERT OR IGNORE INTO players VALUES (?, ?)", player.SteamID64, player.name)
	if err != nil {
		log.Fatal(err)
	}
}

func endParse(gs dem.GameState, round *Round, winningTeam common.Team, db *sql.DB) {
	if round.duration == 0 {
		return
	}
	round.endOfficialTick = gs.IngameTick()

	for i, s := range round.killTicks {
		if i != 0 {
			if s-round.killTicks[i-1] > round.longestKillWait {
				round.longestKillWait = s - round.killTicks[i-1]
			}
		}
		if s <= round.endTick {
			round.lastKillToEnd = round.endTick - s
		}
	}

	switch winningTeam {
	case common.TeamTerrorists:
		for _, s := range gs.TeamCounterTerrorists().Members() {
			if s.IsAlive() {
				round.survivingPlayers = append(round.survivingPlayers, s.SteamID64)
				round.equipmentSavedValue += s.EquipmentValueCurrent()
			}
			round.losingTeamLeftoverMoney += s.Money()
		}
	case common.TeamCounterTerrorists:
		for _, s := range gs.TeamTerrorists().Members() {
			if s.IsAlive() {
				round.survivingPlayers = append(round.survivingPlayers, s.SteamID64)
				round.equipmentSavedValue += s.EquipmentValueCurrent()
			}
			round.losingTeamLeftoverMoney += s.Money()
		}
	}

	survivingPlayersJSON, _ := json.Marshal(round.survivingPlayers)
	killTickJSON, _ := json.Marshal(round.killTicks)
	smokeTickJSON, _ := json.Marshal(round.smokeTicks)
	molotovTickJSON, _ := json.Marshal(round.molotovTicks)
	heTickJSON, _ := json.Marshal(round.heTicks)

	_, err := db.Exec("INSERT INTO round VALUES(NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", round.gameId, round.duration, round.losingTeamName, round.losingSide, round.startTick, round.endTick, round.endOfficialTick, string(survivingPlayersJSON), round.losingTeamLeftoverMoney, round.equipmentSavedValue, string(killTickJSON), string(smokeTickJSON), string(molotovTickJSON), string(heTickJSON), round.longestKillWait, round.lastKillToEnd, round.round, round.heDamage)
	if err != nil {
		log.Fatal(err)
	}
}
