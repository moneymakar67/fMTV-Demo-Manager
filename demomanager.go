package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	dem "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs"
	common "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/common"
	events "github.com/markus-wa/demoinfocs-golang/v4/pkg/demoinfocs/events"
)

// PlayerStats holds the aggregated statistics for a player
type PlayerStats struct {
	Name        string
	SteamID64   uint64
	TeamName    string
	Kills       int
	Assists     int
	Deaths      int
	Damage      int
	RoundsAlive int
	OpeningK    int
	Clutches    int
	HeadshotKills int
	MVPs          int
	
	FirstRound     int
	LastRound      int
	SwitchedTeams  bool
	
	// Temporary tracking for KAST and Multi-Kill metrics
	AliveThisRound    bool
	KillsThisRound    int
	SurvivedRounds    int
	KastKill          bool
	KastAssist        bool
	KastSurvived      bool
	KastTraded        bool
	KastRounds        int
	TradedRounds      int // Simplified: if they didn't survive but got a trade (complex to track perfectly in a short script, so we'll approximate KAST via Survival+Kills)
	
	MultiKills map[int]int // Maps kill count (2,3,4,5) to number of occurrences
	
	// fMR Score
	FMR           float64
}

// Global stats tracker
var playerStats map[uint64]*PlayerStats
var totalRounds int
var team_T string = "Terrorists"
var team_CT string = "Counter-Terrorists"
var score_T int = 0
var score_CT int = 0
var maxParsedRound int = 0

func AnalyzeDemo(filePath string) string {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Sprintf("Error opening %s: %v", filepath.Base(filePath), err)
	}
	defer f.Close()

	parser := dem.NewParser(f)
	defer parser.Close()
	
	playerStats = make(map[uint64]*PlayerStats)
	totalRounds = 0
	maxParsedRound = 0
	
	var lastTeamSideSwitchTick int = -1
	var firstKillThisRound bool
	var potentialClutchers map[common.Team]uint64
	
	// Track when players died to calculate Trades (KAST)
	deathTicks := make(map[uint64]int)
	
	// Register event handlers
	
	// Round Start: Reset temp trackers
	parser.RegisterEventHandler(func(e events.RoundStart) {
		firstKillThisRound = false
		potentialClutchers = make(map[common.Team]uint64)
		for _, p := range playerStats {
			p.KillsThisRound = 0
			p.AliveThisRound = true
			p.KastKill = false
			p.KastAssist = false
			p.KastSurvived = true
			p.KastTraded = false
		}
		deathTicks = make(map[uint64]int)

		// Check for 1vX situations at start of round
		for _, team := range []common.Team{common.TeamTerrorists, common.TeamCounterTerrorists} {
			otherTeam := common.TeamCounterTerrorists
			if team == common.TeamCounterTerrorists { otherTeam = common.TeamTerrorists }
			
			aliveCount := 0
			var lastAlive uint64 = 0
			for _, p := range parser.GameState().Participants().Playing() {
				if p != nil && p.Team == team && p.IsAlive() {
					aliveCount++
					lastAlive = p.SteamID64
				}
			}
			
			otherAlive := 0
			for _, p := range parser.GameState().Participants().Playing() {
				if p != nil && p.Team == otherTeam && p.IsAlive() {
					otherAlive++
				}
			}
			
			if aliveCount == 1 && lastAlive != 0 && otherAlive > 0 {
				potentialClutchers[team] = lastAlive
				fmt.Printf("[Stats] Potential Clutch: Player on team %v started round in 1v%d situation\n", team, otherAlive)
			}
		}
	})
	
	// Player Death: Track Kills, Assists, Deaths
	parser.RegisterEventHandler(func(e events.Kill) {
		// Track Opening Kill
		if !firstKillThisRound && e.Killer != nil && e.Killer.SteamID64 != 0 {
			firstKillThisRound = true
			if p, ok := playerStats[e.Killer.SteamID64]; ok {
				p.OpeningK++
			}
		}

		// Track Death
		if e.Victim != nil && e.Victim.SteamID64 != 0 {
			id := e.Victim.SteamID64
			initPlayer(e.Victim)
			if p, ok := playerStats[id]; ok {
				p.Deaths++
				p.AliveThisRound = false
				p.KastSurvived = false
			}
			
			// Detect Clutch logic (Check both teams)
			for _, t := range []common.Team{common.TeamTerrorists, common.TeamCounterTerrorists} {
				otherT := common.TeamCounterTerrorists
				if t == common.TeamCounterTerrorists { otherT = common.TeamTerrorists }
				
				aliveCount := 0
				var lastAlive uint64 = 0
				for _, p := range parser.GameState().Participants().Playing() {
					if p != nil && p.Team == t && p.IsAlive() {
						aliveCount++
						lastAlive = p.SteamID64
					}
				}
				
				otherAlive := 0
				for _, p := range parser.GameState().Participants().Playing() {
					if p != nil && p.Team == otherT && p.IsAlive() {
						otherAlive++
					}
				}
				
				if aliveCount == 1 && lastAlive != 0 && otherAlive > 0 {
					if _, exists := potentialClutchers[t]; !exists {
						potentialClutchers[t] = lastAlive
						fmt.Printf("[Stats] Potential Clutch: Player on team %v is now in 1v%d situation\n", t, otherAlive)
					}
				}
			}
		}
		
		// Track Kill
		if e.Killer != nil && e.Killer.SteamID64 != 0 {
			id := e.Killer.SteamID64
			initPlayer(e.Killer)
			if p, ok := playerStats[id]; ok {
				p.Kills++
				p.KillsThisRound++
				p.KastKill = true
				if e.IsHeadshot {
					p.HeadshotKills++
				}
			}
		}
		
		// Track Assist
		if e.Assister != nil && e.Assister.SteamID64 != 0 {
			id := e.Assister.SteamID64
			initPlayer(e.Assister)
			if p, ok := playerStats[id]; ok {
				p.Assists++
				p.KastAssist = true
			}
		}

		// KAST: Trade Logic
		// Record exact death tick for the Victim
		if e.Victim != nil && e.Victim.SteamID64 != 0 {
			deathTicks[e.Victim.SteamID64] = parser.CurrentFrame()
		}
		
		// If the Killer got a kill, check if any of their teammates died in the last ~5 seconds (400 ticks). 
		// If so, those teammates were "Traded".
		if e.Killer != nil && e.Victim != nil && e.Killer.TeamState != nil {
			killerTeamStr := ""
			if e.Killer.Team == 2 { killerTeamStr = "Terrorists" } else if e.Killer.Team == 3 { killerTeamStr = "Counter-Terrorists" }
			
			for deadID, deadTick := range deathTicks {
				if parser.CurrentFrame()-deadTick <= 400 {
					if deadPlayerStat, ok := playerStats[deadID]; ok && deadPlayerStat.TeamName == killerTeamStr {
						deadPlayerStat.KastTraded = true
					}
				}
			}
		}
	})
	
	// Track MVPs
	parser.RegisterEventHandler(func(e events.RoundMVPAnnouncement) {
		if e.Player != nil && e.Player.SteamID64 != 0 {
			id := e.Player.SteamID64
			initPlayer(e.Player)
			if p, ok := playerStats[id]; ok {
				p.MVPs++
			}
		}
	})
	
	// Damage
	parser.RegisterEventHandler(func(e events.PlayerHurt) {
		// e.Attacker or e.Player can be nil (e.g. falling damage, world damage)
		if e.Attacker != nil && e.Player != nil && e.Attacker.SteamID64 != 0 {
			// Add safe guard for TeamState and pointers
			if e.Attacker.TeamState != nil && e.Player.TeamState != nil {
				if e.Attacker.Team != e.Player.Team {
					id := e.Attacker.SteamID64
					initPlayer(e.Attacker)
					if p, ok := playerStats[id]; ok {
						p.Damage += e.HealthDamage
					}
				}
			}
		}
	})
	
	// Round End: Accumulate Multi-Kills, Survivals, Round Count
	parser.RegisterEventHandler(func(e events.RoundEnd) {
		totalRounds++
		for _, p := range playerStats {
			if p.AliveThisRound {
				p.SurvivedRounds++
			}
			
			// KAST Check: Kill, Assist, Survive, or Trade
			if p.KastKill || p.KastAssist || p.KastSurvived || p.KastTraded {
				p.KastRounds++
			}

			// Multi-kills
			if p.KillsThisRound >= 2 {
				if p.MultiKills == nil {
					p.MultiKills = make(map[int]int)
				}
				p.MultiKills[p.KillsThisRound]++
			}
		}
		
		maxParsedRound = totalRounds
		
		for _, player := range parser.GameState().Participants().Playing() {
			if player != nil && player.SteamID64 != 0 && !player.IsBot {
				initPlayer(player)
				stats := playerStats[player.SteamID64]
				if stats.FirstRound == -1 {
					stats.FirstRound = totalRounds
				}
				stats.LastRound = totalRounds
			}
		}
		
		// Award Clutches
		if e.Winner == common.TeamTerrorists || e.Winner == common.TeamCounterTerrorists {
			if clutcherID, exists := potentialClutchers[e.Winner]; exists {
				if p, ok := playerStats[clutcherID]; ok {
					p.Clutches++
				}
			}
		}
		
		// Update Team Scores based on the current parser state + the current round's winner
		// because GameState().Score() is not formally updated until ScoreUpdated event fires slightly later.
		gameTime := parser.GameState()
		sT := gameTime.TeamTerrorists().Score()
		sCT := gameTime.TeamCounterTerrorists().Score()
		
		if e.Winner == common.TeamTerrorists {
			sT++
		} else if e.Winner == common.TeamCounterTerrorists {
			sCT++
		}
		
		score_T = sT
		score_CT = sCT
	})

	parser.RegisterEventHandler(func(e events.TeamSideSwitch) {
		lastTeamSideSwitchTick = parser.CurrentFrame()
	})

	parser.RegisterEventHandler(func(e events.PlayerTeamChange) {
		if e.Player != nil && e.Player.SteamID64 != 0 && !e.Player.IsBot {
			id := e.Player.SteamID64
			initPlayer(e.Player)
			if p, ok := playerStats[id]; ok {
				// We only care if they switched between T and CT, not spectator joining
				oldValid := e.OldTeam == common.TeamTerrorists || e.OldTeam == common.TeamCounterTerrorists
				newValid := e.NewTeam == common.TeamTerrorists || e.NewTeam == common.TeamCounterTerrorists
				
				// Ignore if this change happened precisely during a halftime mass-swap
				if parser.CurrentFrame() != lastTeamSideSwitchTick {
					if oldValid && newValid && e.OldTeam != e.NewTeam {
						p.SwitchedTeams = true
					}
				}
				
				// Update their final team designation
				if e.NewTeam == common.TeamTerrorists {
					p.TeamName = "Terrorists"
				} else if e.NewTeam == common.TeamCounterTerrorists {
					p.TeamName = "Counter-Terrorists"
				}
			}
		}
	})

	// Parse the whole demo
	err = parser.ParseToEnd()
	if err != nil {
		log.Printf("Parsing stopped with error (often fine at end of demo): %v\n", err)
	}
	
	// Truncation fix: if the demo recording cuts off before the final round finishes,
	// artificially award the 16th point and increment the round count (excluding ties).
	if score_T == 15 && score_CT < 15 {
		score_T = 16
		totalRounds++
	} else if score_CT == 15 && score_T < 15 {
		score_CT = 16
		totalRounds++
	}
	
	header := parser.Header()
	mapName := header.MapName
	serverName := header.ServerName
	
	// Finalize fMR Math
	calculateFMR()
	
	// Generate Report
	errReport := generateHTMLReport(filePath, mapName, serverName)
	if errReport != "" {
		return errReport
	}
	
	return fmt.Sprintf("Analysis complete for %s! Output saved.", mapName)
}

func initPlayer(p *common.Player) {
	if p == nil || p.IsBot || p.SteamID64 == 0 {
		return
	}
	id := p.SteamID64
	if _, exists := playerStats[id]; !exists {
		team := "Spectator"
		// Prevent invalid memory access on nil team state
		if p.TeamState != nil {
			if p.Team == 2 {
				team = "Terrorists"
			} else if p.Team == 3 {
				team = "Counter-Terrorists"
			}
		}
		
		name := p.Name
		if name == "" {
			name = "Unknown Player"
		}
		
		playerStats[id] = &PlayerStats{
			Name:       name,
			SteamID64:  id,
			TeamName:   team,
			MultiKills: make(map[int]int),
			FirstRound: -1,
			LastRound:  -1,
		}
	}
}

func clamp(val, min, max float64) float64 {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

func calculateFMR() {
	if totalRounds == 0 {
		return
	}
	
	tr := float64(totalRounds)
	
	for _, p := range playerStats {
		// Base metrics
		kpr := float64(p.Kills) / tr
		apr := float64(p.Assists) / tr
		dpr := float64(p.Deaths) / tr
		adr := float64(p.Damage) / tr
		okpr := float64(p.OpeningK) / tr
		
		// Use precise KAST tracking
		kast := (float64(p.KastRounds) / tr) * 100.0
		
		// Refined Multi-Kill weight
		mk := float64(p.MultiKills[2])*0.05 + float64(p.MultiKills[3])*0.12 + float64(p.MultiKills[4])*0.25 + float64(p.MultiKills[5])*0.50
		
		// Clutch weight (Accurate 1vX wins)
		cl := float64(p.Clutches)
		
		// Impact Formula: Impact = (2.13*KPR) + (0.42*APR) + (0.35*OKPR) + (0.25*CL) + MK - 0.41
		impact := (2.13 * kpr) + (0.42 * apr) + (0.35 * okpr) + (0.25 * cl) + mk - 0.41
		
		// Quality Q metric (assuming 1.0 standard for now without deep econ tracking)
		q := 1.0 
		
		// fMR Formula: 0.0073*KAST + 0.3591*(KPR*Q) - 0.5329*DPR + 0.2372*Impact + 0.0032*ADR + 0.1587
		fmr := (0.0073 * kast) + (0.3591 * (kpr * q)) - (0.5329 * dpr) + (0.2372 * impact) + (0.0032 * adr) + 0.1587
		
		p.FMR = fmr
	}
}

func generateHTMLReport(filePath, mapName, serverName string) string {
	outPath := filePath + ".report.html"
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Sprintf("Error creating HTML: %v", err)
	}
	defer f.Close()
	
	logoPath := "fragMount_white_nobg.svg"
	logoData, errLogo := os.ReadFile(logoPath)
	logoBase64 := ""
	if errLogo == nil {
		logoBase64 = base64.StdEncoding.EncodeToString(logoData)
	}
	
	var terrorists []*PlayerStats
	var cts []*PlayerStats
	var specs []*PlayerStats
	
	for _, p := range playerStats {
		if p.Name == "" {
			continue
		}
		if p.TeamName == "Terrorists" {
			terrorists = append(terrorists, p)
		} else if p.TeamName == "Counter-Terrorists" {
			cts = append(cts, p)
		} else {
			specs = append(specs, p)
		}
	}
	
	sortByFMR := func(i, j int, list []*PlayerStats) bool {
		return list[i].FMR > list[j].FMR
	}
	sort.Slice(terrorists, func(i, j int) bool { return sortByFMR(i, j, terrorists) })
	sort.Slice(cts, func(i, j int) bool { return sortByFMR(i, j, cts) })
	
	html := fmt.Sprintf(`
	<!DOCTYPE html>
	<html>
	<head>
		<title>fMR Match Report</title>
		<style>
			body { font-family: 'Inter', sans-serif; background: #000; color: #fff; padding: 2rem; margin: 0; }
			.container { max-width: 1000px; margin: 0 auto; background: #000; padding: 2rem; border-radius: 8px; border: 1px solid #fff; }
			h1 { color: #fff; display: flex; align-items: center; gap: 15px; margin-top: 0; }
			.meta { color: #fff; margin-bottom: 2rem; }
			table { width: 100%%; border-collapse: collapse; margin-top: 1rem; }
			th, td { text-align: left; padding: 0.75rem; border-bottom: 1px solid #fff; }
			th { color: #fff; font-weight: 500; font-size: 0.85rem; text-transform: uppercase; }
			.fmr { font-weight: bold; color: #fff; }
			.fmr-container { display: flex; align-items: center; gap: 6px; }
		</style>
	</head>
	<body>
		<div class="container">
			<h1>
				<img src="data:image/svg+xml;base64,%s" style="width: 62px; height: 67px;" alt="Logo">
				FMTV Demo Report
			</h1>
			<div class="meta">
				<p>File: %s</p>
				<p>Map: <strong>%s</strong> | Server: %s | Rounds: %d</p>
				<p>Score: <span style="color: #eab308">Terrorists: %d</span> — <span style="color: #60a5fa">CTs: %d</span></p>
			</div>
	`, logoBase64, filepath.Base(filePath), mapName, serverName, totalRounds, score_T, score_CT)
	
	// Track if legend is needed
	hasRinger := false
	hasLeaver := false
	hasSwapper := false
	hasMVP := false
	
	// Helper string builder for tables
	buildTable := func(teamName string, list []*PlayerStats, colorClass string, isWinningTeam bool) {
		html += fmt.Sprintf(`
			<h2 style="margin-top: 2rem; color: %s">%s</h2>
			<table>
				<tr>
					<th>Player</th>
					<th>K</th>
					<th>A</th>
					<th>D</th>
					<th>K/D</th>
					<th>K/R</th>
					<th>KAST</th>
					<th>HS%%</th>
					<th>Entry</th>
					<th>Clutch</th>
					<th>MVPs</th>
					<th>3K</th>
					<th>4K</th>
					<th>5K</th>
					<th>ADR</th>
					<th>fMR</th>
				</tr>
		`, colorClass, teamName)
		
		for i, p := range list {
			adr := float64(p.Damage) / float64(totalRounds)
			if totalRounds == 0 { adr = 0 }
			
			flags := ""
			// If they weren't there for round 1
			if p.FirstRound > 1 {
				hasRinger = true
				flags += ` <span style="color: #eab308; font-size: 0.8rem; font-weight: bold;" title="Ringer (Joined Late)">(R)</span>`
			}
			// If they weren't there for the final parsed round
			if p.LastRound < maxParsedRound {
				hasLeaver = true
				flags += ` <span style="color: #ef4444; font-size: 0.8rem; font-weight: bold;" title="Leaver (Left Early)">(L)</span>`
			}
			// If they switched teams mid-match
			if p.SwitchedTeams {
				hasSwapper = true
				flags += ` <span style="color: #a855f7; font-size: 0.8rem; font-weight: bold;" title="Switched Teams">(S)</span>`
			}
			
			// Match MVP logic
			mvpFlag := ""
			if isWinningTeam && i == 0 && len(list) > 0 {
				hasMVP = true
				mvpFlag = `<span style="color: #eab308; font-size: 0.9em; font-weight: bold; transform: translateY(-0.12em); display: inline-block;" title="Match MVP">★</span>`
			}
			
			kdRatio := 0.0
			if p.Deaths > 0 {
				kdRatio = float64(p.Kills) / float64(p.Deaths)
			} else {
				kdRatio = float64(p.Kills)
			}
			
			krRatio := 0.0
			if totalRounds > 0 {
				krRatio = float64(p.Kills) / float64(totalRounds)
			}
			
			hsPercent := 0.0
			if p.Kills > 0 {
				hsPercent = (float64(p.HeadshotKills) / float64(p.Kills)) * 100.0
			}

			kastPercent := 0.0
			if totalRounds > 0 {
				kastPercent = (float64(p.KastRounds) / float64(totalRounds)) * 100.0
			}
			
			html += fmt.Sprintf(`
				<tr>
					<td><strong><a href="https://steamcommunity.com/profiles/%d" target="_blank" style="color: inherit; text-decoration: none; border-bottom: 1px dashed rgba(255,255,255,0.4);">%s</a></strong>%s</td>
					<td>%d</td>
					<td>%d</td>
					<td>%d</td>
					<td>%.2f</td>
					<td>%.2f</td>
					<td>%.1f%%</td>
					<td>%.1f%%</td>
					<td>%d</td>
					<td>%d</td>
					<td>%d</td>
					<td>%d</td>
					<td>%d</td>
					<td>%d</td>
					<td>%.1f</td>
					<td class="fmr"><div class="fmr-container">%.2f%s</div></td>
				</tr>
			`, p.SteamID64, p.Name, flags, p.Kills, p.Assists, p.Deaths, kdRatio, krRatio, kastPercent, hsPercent, p.OpeningK, p.Clutches, p.MVPs, p.MultiKills[3], p.MultiKills[4], p.MultiKills[5], adr, p.FMR, mvpFlag)
		}
		html += `</table>`
	}
	
	// Render Winner First
	if score_T > score_CT {
		buildTable("Terrorists", terrorists, "#eab308", true)
		buildTable("Counter-Terrorists", cts, "#60a5fa", false)
	} else if score_CT > score_T {
		buildTable("Counter-Terrorists", cts, "#60a5fa", true)
		buildTable("Terrorists", terrorists, "#eab308", false)
	} else {
		// Draw
		buildTable("Counter-Terrorists", cts, "#60a5fa", false)
		buildTable("Terrorists", terrorists, "#eab308", false)
	}

	if hasRinger || hasLeaver || hasSwapper || hasMVP {
		html += `<div style="margin-top: 2rem; padding: 1rem; background: #000; border: 1px solid #fff; border-radius: 4px; font-size: 0.85rem; color: #fff;">`
		html += `<strong style="color: #fff; display: block; margin-bottom: 0.5rem;">Legend</strong>`
		if hasMVP {
			html += `<span style="color: #eab308; font-weight: bold;">★</span> — Match MVP (Highest fMR on the winning team)<br>`
		}
		if hasRinger {
			html += `<span style="color: #eab308; font-weight: bold;">(R)</span> — Ringer (Joined match after Round 1 and substituted for a leaver)<br>`
		}
		if hasLeaver {
			html += `<span style="color: #ef4444; font-weight: bold;">(L)</span> — Leaver (Abandoned match before final round concluded)<br>`
		}
		if hasSwapper {
			html += `<span style="color: #a855f7; font-weight: bold;">(S)</span> — Switched Teams (Voluntarily changed sides mid-match)`
		}
		html += `</div>`
	}

	html += `
		</div>
	</body>
	</html>
	`
	
	f.WriteString(html)
	return fmt.Sprintf("Analysis complete for %s! Output saved.", mapName)
}
