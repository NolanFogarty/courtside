package backend

import "strings"

// Team uses the NBA's stable franchise ID and ESPN's independent team slug.
type Team struct {
	ID               int
	Name, Code, ESPN string
}

// ConferenceForTeam reports which conference a team plays in ("east" or
// "west"). It accepts either the full name ("Boston Celtics") or the nickname
// ("Celtics"), matching case-insensitively. Unknown names return "".
func ConferenceForTeam(name string) string {
	norm := strings.ToLower(strings.TrimSpace(name))
	for nick, conf := range teamConferences {
		if norm == nick || strings.HasSuffix(norm, " "+nick) {
			return conf
		}
	}
	return ""
}

// GameConference classifies a game by its home team's conference, falling back
// to the away team when the home side is unrecognized. Games where neither
// side is recognized balance into the shorter column so none are dropped.
func GameConference(g Game) string {
	if conf := ConferenceForTeam(g.HomeTeam); conf != "" {
		return conf
	}
	return ConferenceForTeam(g.AwayTeam)
}

var teamConferences = map[string]string{
	"hawks": "east", "celtics": "east", "nets": "east", "hornets": "east",
	"bulls": "east", "cavaliers": "east", "pistons": "east", "pacers": "east",
	"heat": "east", "bucks": "east", "knicks": "east", "magic": "east",
	"76ers": "east", "raptors": "east", "wizards": "east",
	"mavericks": "west", "nuggets": "west", "warriors": "west",
	"rockets": "west", "clippers": "west", "lakers": "west",
	"grizzlies": "west", "pelicans": "west", "timberwolves": "west",
	"thunder": "west", "suns": "west", "trail blazers": "west",
	"kings": "west", "spurs": "west", "jazz": "west",
}

var Teams = []Team{
	{1610612737, "Atlanta Hawks", "ATL", "atl"},
	{1610612738, "Boston Celtics", "BOS", "bos"},
	{1610612751, "Brooklyn Nets", "BKN", "bkn"},
	{1610612766, "Charlotte Hornets", "CHA", "cha"},
	{1610612741, "Chicago Bulls", "CHI", "chi"},
	{1610612739, "Cleveland Cavaliers", "CLE", "cle"},
	{1610612742, "Dallas Mavericks", "DAL", "dal"},
	{1610612743, "Denver Nuggets", "DEN", "den"},
	{1610612765, "Detroit Pistons", "DET", "det"},
	{1610612744, "Golden State Warriors", "GSW", "gs"},
	{1610612745, "Houston Rockets", "HOU", "hou"},
	{1610612754, "Indiana Pacers", "IND", "ind"},
	{1610612746, "LA Clippers", "LAC", "lac"},
	{1610612747, "Los Angeles Lakers", "LAL", "lal"},
	{1610612763, "Memphis Grizzlies", "MEM", "mem"},
	{1610612748, "Miami Heat", "MIA", "mia"},
	{1610612749, "Milwaukee Bucks", "MIL", "mil"},
	{1610612750, "Minnesota Timberwolves", "MIN", "min"},
	{1610612740, "New Orleans Pelicans", "NOP", "no"},
	{1610612752, "New York Knicks", "NYK", "ny"},
	{1610612760, "Oklahoma City Thunder", "OKC", "okc"},
	{1610612753, "Orlando Magic", "ORL", "orl"},
	{1610612755, "Philadelphia 76ers", "PHI", "phi"},
	{1610612756, "Phoenix Suns", "PHX", "phx"},
	{1610612757, "Portland Trail Blazers", "POR", "por"},
	{1610612758, "Sacramento Kings", "SAC", "sac"},
	{1610612759, "San Antonio Spurs", "SAS", "sa"},
	{1610612761, "Toronto Raptors", "TOR", "tor"},
	{1610612762, "Utah Jazz", "UTA", "utah"},
	{1610612764, "Washington Wizards", "WAS", "wsh"},
}
