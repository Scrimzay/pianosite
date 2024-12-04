package main

import (
	"fmt"
	"math"
	"sync"
	"math/rand"
	"net/http"
	"encoding/json"
	"log"

	"github.com/hajimehoshi/oto"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
    sampleRate = 44100 // CD-quality sample rate
    duration   = 0.3  // Duration in seconds
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func generateMelodicFrequencies() map[rune]float64 {
	baseFreq := 130.81 // Start at C3
	keyOrder := []rune{'a', 's', 'd', 'f', 'g', 'h', 'j', 'k', 'l', ';', '\'', 'e', 'r', 't', 'u', 'i', 'p', '[', ']', '\\'}
	frequencies := make(map[rune]float64)
	startNote := 6

	for i, key := range keyOrder {
		// Calc frequency using the formula
		frequencies[key] = baseFreq * math.Pow(2, float64(startNote+i)/12)
	}

	return frequencies
}

func generateSoftWave(freq float64, durationSec float64, sampleRate int, waveType string) []byte {
    samples := int(float64(sampleRate) * durationSec)
    buf := make([]byte, samples)

    for i := 0; i < samples; i++ {
        t := float64(i) / float64(sampleRate)

		// different waves for different instrument types
        sine := math.Sin(2 * math.Pi * freq * t)
        square := math.Copysign(1, sine)
        triangle := math.Asin(sine) / (math.Pi / 2)
        sawtooth := 2*(t*freq - math.Floor(0.5+t*freq))
        noise := 2*rand.Float64() - 1 // random value between -1 and 1

		// Instrument waves
        var wave float64
        switch waveType {
        case "sine":
            wave = sine
        case "square":
            wave = square
        case "triangle":
            wave = triangle
        case "sawtooth":
            wave = sawtooth
        case "noise":
            wave = noise
        case "flute":
            wave = math.Sin(2 * math.Pi * freq * t)
        case "clarinet":
            wave = 0.7*math.Sin(2*math.Pi*freq*t) + 0.3*math.Copysign(1, math.Sin(2*math.Pi*freq*t))
        case "organ":
            wave = 0.5*math.Sin(2*math.Pi*freq*t) + 0.5*triangle
        case "strings":
            wave = 0.6*math.Sin(2*math.Pi*freq*t) + 0.4*sawtooth
        case "synth":
            wave = 0.4*square + 0.4*sawtooth + 0.2*math.Sin(2*math.Pi*freq*t)
        case "piano":
            wave = 0.8*math.Sin(2*math.Pi*freq*t) + 0.2*noise
        case "chiptune":
            wave = math.Copysign(1, math.Sin(2*math.Pi*freq*t))
        default:
            wave = sine // Default to sine wave if invalid waveType
        }
		
		// Apply amplitude modulation for a fade-out effect
		amplitude := 0.8 * (1 - float64(i) / float64(samples)) // Linear fade
		sample := 128 + int(127 * amplitude * wave)

		// Clamp to valid byte range
		if sample > 255 {
			sample = 192
		} else if sample < 0 {
			sample = 0
		}

		buf[i] = byte(sample)
    }

    return buf
}

func main() {
	frequencies := generateMelodicFrequencies()
	context, err := oto.NewContext(sampleRate, 1, 1, 4096)
	if err != nil {
		panic(err)
	}
	defer context.Close()

	player := context.NewPlayer()
	defer player.Close()

	playbackQueue := make(chan []byte, 10)
	wg := &sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for sound := range playbackQueue {
			player.Write(sound)
		}
	}()

	r := gin.Default()

	r.LoadHTMLGlob("*.html")
	r.Static("/static", "./static")

	r.GET("/", func(ctx *gin.Context) {
		ctx.HTML(200, "index.html", nil)
	})
	r.GET("/ws", func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			fmt.Println("Failed to upgrade to WebSocket:", err)
			return
		}
		defer conn.Close()

		var waveType = "strings" // Default wave type
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				fmt.Println("Read error:", err)
				break
			}
		
			// Attempt to parse as JSON
			var msg struct {
				Type string `json:"type"`
				Key  string `json:"key,omitempty"`
				Wave string `json:"wave,omitempty"`
			}
		
			if err := json.Unmarshal(message, &msg); err != nil {
				// If parsing fails, treat the message as a single key press
				if len(message) == 1 { // Ensure it's a single character
					key := []rune(string(message))[0]
					if freq, exists := frequencies[key]; exists {
						sound := generateSoftWave(freq, duration, sampleRate, waveType)
						select {
						case playbackQueue <- sound:
						default:
							fmt.Println("Playback queue full, sound skipped")
						}
					} else {
						fmt.Println("Unknown key pressed:", string(message))
					}
				} else {
					fmt.Printf("Invalid message format: %v\n", err)
				}
				continue
			}
		
			// Process valid JSON messages
			switch msg.Type {
			case "wave":
				if msg.Wave != "" {
					waveType = msg.Wave
					fmt.Println("Wave type updated to:", waveType)
				} else {
					fmt.Println("Empty wave type received")
				}
			case "key":
				if len(msg.Key) == 1 { // Validate single character key
					key := []rune(msg.Key)[0]
					if freq, exists := frequencies[key]; exists {
						sound := generateSoftWave(freq, duration, sampleRate, waveType)
						select {
						case playbackQueue <- sound:
						default:
							fmt.Println("Playback queue full, sound skipped")
						}
					} else {
						fmt.Println("Unknown key pressed:", msg.Key)
					}
				} else {
					fmt.Println("Invalid key format:", msg.Key)
				}
			default:
				fmt.Println("Unknown message type:", msg.Type)
			}
		}
	})
	
	err = r.Run(":8080")
	if err != nil {
		log.Fatal(err)
	}
}