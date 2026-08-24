package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"example.com/fastly-soundscape/internal/fastly"
	"example.com/fastly-soundscape/internal/output"
	"example.com/fastly-soundscape/internal/synth"
	"example.com/fastly-soundscape/internal/theme"
)

func main() {
	var (
		serviceID = flag.String("service-id", os.Getenv("FASTLY_SERVICE_ID"), "Fastly service ID")
		token     = flag.String("token", os.Getenv("FASTLY_API_TOKEN"), "Fastly API token")
		themePath = flag.String("theme", "themes/forest.yaml", "theme YAML file")
		soundFont = flag.String("soundfont", "", "optional SF2 SoundFont")
		simulate  = flag.Bool("simulate", false, "use generated telemetry instead of Fastly")
		verbose   = flag.Bool("verbose", false, "log telemetry")
	)
	flag.Parse()

	th, err := theme.Load(*themePath)
	if err != nil {
		log.Fatal(err)
	}

	var out output.Output = output.NewConsole()

	if *soundFont != "" {
		sf, err := synth.NewSoundFontOutput(*soundFont)
		if err != nil {
			log.Fatal(err)
		}
		defer sf.Close()
		out = sf
	}

	engine := theme.NewEngine(th, out)

	if *simulate {
		runSimulation(engine)
		return
	}

	if *serviceID == "" || *token == "" {
		log.Fatal("FASTLY_API_TOKEN and --service-id are required unless --simulate is used")
	}

	client := fastly.NewClient(*token, *serviceID)

	var timestamp int64
	for {
		resp, err := client.Fetch(timestamp)
		if err != nil {
			log.Printf("Fastly: %v", err)
			time.Sleep(time.Second)
			continue
		}

		if *verbose {
			fmt.Printf("timestamp=%d records=%d delay=%ds\n",
				resp.Timestamp, len(resp.Data), resp.AggregateDelay)
		}

		for _, record := range resp.Data {
			engine.Process(record.Recorded, record.Aggregated)
		}

		timestamp = resp.Timestamp
		time.Sleep(200 * time.Millisecond)
	}
}

func runSimulation(engine *theme.Engine) {
	// A deliberately slow cycle: quiet -> busy -> quiet.
	var t float64
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		requests := 50.0 + (1+math.Sin(t))*4500.0
		bandwidth := requests * (1000 + 8000*(1+math.Sin(t*0.7))/2)
		errors := 1.0 + 10.0*(1+math.Sin(t*1.7))/2

		engine.Process(int64(time.Now().Unix()), map[string]float64{
			"requests":        requests,
			"resp_body_bytes": bandwidth,
			"errors":          errors,
			"hits":            requests * 0.85,
		})

		t += 0.12
	}
}
