package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"os"
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
	survivingPlayers        []string
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

type Game struct {
	TeamA        string
	TeamAPlayers []string
	TeamB        string
	TeamBPlayers []string
	date         time.Time
	de_map       string
}

func main() {
	const db_file string = "csgo.db"
	db, err := sql.Open("sqlite3", db_file)
	if err != nil {
		log.Panic(err)
	}

	parseDemo("../demos/ESLOneCologne-GF-nip-vs-fnatic/ESLOneCologne-GF-fnatic-vs-nip-cache.dem", db)
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
		startParse(p.GameState(), &round, &game, db, &gameId, p.Header().MapName)

	})

	p.RegisterEventHandler(func(e events.GamePhaseChanged) {
		if e.NewGamePhase == common.GamePhaseGameEnded {
			endParse(p.GameState(), &round, winningTeam, db)
		} else if e.NewGamePhase == common.GamePhaseStartGamePhase {
			startParse(p.GameState(), &round, &game, db, &gameId, p.Header().MapName)
		}
	})

	p.RegisterEventHandler(func(e events.RoundFreezetimeEnd) {
		round.startTick = p.GameState().IngameTick()
	})

	p.RegisterEventHandler(func(e events.RoundEnd) {
		gs := p.GameState()

		round.endTick = gs.IngameTick()
		round.duration = (round.endTick - round.startTick) / int(p.TickRate())

		winningTeam = e.Winner

		round.losingSide = int(e.WinnerState.Opponent.ID())
		round.losingTeamName = e.WinnerState.Opponent.ClanName()
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

func startParse(gs dem.GameState, round *Round, game *Game, db *sql.DB, gameId *int64, de_map string) {
	round.round = gs.TotalRoundsPlayed()

	if len(game.TeamAPlayers) == 0 {
		for _, s := range gs.TeamCounterTerrorists().Members() {
			game.TeamAPlayers = append(game.TeamAPlayers, s.Name)
		}
		for _, s := range gs.TeamTerrorists().Members() {
			game.TeamBPlayers = append(game.TeamBPlayers, s.Name)
		}
		game.TeamA = gs.TeamCounterTerrorists().ClanName()
		game.TeamB = gs.TeamTerrorists().ClanName()

		game.de_map = de_map

		TeamAPlayersJson, _ := json.Marshal(game.TeamAPlayers)
		TeamBPlayersJson, _ := json.Marshal(game.TeamBPlayers)

		res, err := db.Exec("INSERT INTO game VALUES(NULL, ?, ?, ?, ?, ?, ?)", game.date.Format(time.RFC3339), game.TeamA, game.TeamB, string(TeamAPlayersJson), string(TeamBPlayersJson), de_map)
		if err != nil {
			log.Fatal(err)
		}

		if *gameId, err = res.LastInsertId(); err != nil {
			log.Fatal(err)
		}
	}
	round.gameId = *gameId
}

func endParse(gs dem.GameState, round *Round, winningTeam common.Team, db *sql.DB) {
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
				round.survivingPlayers = append(round.survivingPlayers, s.Name)
				round.equipmentSavedValue += s.EquipmentValueCurrent()
			}
			round.losingTeamLeftoverMoney += s.Money()
		}
	case common.TeamCounterTerrorists:
		for _, s := range gs.TeamTerrorists().Members() {
			if s.IsAlive() {
				round.survivingPlayers = append(round.survivingPlayers, s.Name)
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
