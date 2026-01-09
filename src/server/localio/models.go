package localio

type ModelSpec struct {
	Name string `json:"name"`
	DI   int    `json:"di"`
	DO   int    `json:"do"`
	AI   int    `json:"ai"`
	AO   int    `json:"ao"`
}

var ModelTable = map[string]ModelSpec{
	"IO0404": {Name: "IO0404", DI: 0, DO: 0, AI: 4, AO: 4},
	"IO0440": {Name: "IO0440", DI: 0, DO: 4, AI: 4, AO: 0},
	"IO4040": {Name: "IO4040", DI: 4, DO: 4, AI: 0, AO: 0},
	"IO8000": {Name: "IO8000", DI: 8, DO: 0, AI: 0, AO: 0},
	"IO0080": {Name: "IO0080", DI: 0, DO: 8, AI: 0, AO: 0},
}

// guessModel mirrors read_di.go mapping
func guessModel(di, doCount, ai, ao int) string {
	switch {
	case di == 4 && doCount == 4 && ai == 0 && ao == 0:
		return "IO4040"
	case di == 0 && doCount == 4 && ai == 4 && ao == 0:
		return "IO0440"
	case di == 0 && doCount == 8 && ai == 0 && ao == 0:
		return "IO0080"
	case di == 8 && doCount == 0 && ai == 0 && ao == 0:
		return "IO8000"
	case di == 0 && doCount == 0 && ai == 4 && ao == 4:
		return "IO0404"
	default:
		return "Unknown"
	}
}
