// Fronton 2D game with multiple balls (3 or more), similar to famous Pong.
// This is a variation written for mobile devices, considering the size and geometry.
//
// Divertimento in go with ebiten, compiled to wasm (WebAssembly).
// Thanks to the GO team, the library of Hajimehoshi and Pixabay sounds.
// Marc Riart, 202405.

//go:build js && wasm

package main

import (
	"embed"
	"fmt"
	"image/color"
	_ "image/png"
	"math/rand/v2"
	"syscall/js"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/mp3"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

var (
	// Screen geometry. It is var (not const) because it adapts to screen device
	screenWidth  int
	screenHeight int
	fieldWidth   int
	fieldHeight  int
	racketWidth  int
	racketHeight int = 5 // +5 pixels of distance between racket and floor, fixed
	racketSpeed  int = 8
	winnerScore  int = 21

	// Used floats in some functions. Precalculated to alleviate cpu
	fieldWidthF   float32
	fieldWidthFF  float64
	fieldHeightF  float32
	fieldHeightFF float64
	racketWidthF  float32
	racketHeightF float32

	// Embeding all resources for portability
	//go:embed res/*
	embeddedFS embed.FS

	arrowRight *ebiten.Image
	arrowLeft  *ebiten.Image

	touchIDs        = []ebiten.TouchID{}
	touchRight bool = false
	touchLeft  bool = false

	fontFaceSource *text.GoTextFaceSource

	// Time measures
	startMatch time.Time
	endMatch   time.Time
	deltaMatch float64

	startLiftFinger time.Time
)

const (
	audioStart = iota
	audioHit
	audioMiss
	audioOver
)

type Game struct {
	State int // Defines the game state: 0 not initiated, 1 started, 2 over
	Score struct {
		Player int
		CPU    int
	}
	Balls  []Ball
	Racket struct {
		X     int // The x point of the vertex top left
		Y     int // The y point of the vertex. It is a fixed value, but useful for geometry
		Speed int
	}
	audioContext     *audio.Context
	audioPlayerStart *audio.Player
	audioPlayerHit   *audio.Player
	audioPlayerMiss  *audio.Player
	audioPlayerOver  *audio.Player
}

type Ball struct {
	Radius int
	Color  color.RGBA
	X      int
	Y      int
	SpeedX int
	SpeedY int
}

func (g *Game) Initialize() {
	// Define the geometry adapted to the device screen size, taken from JS in index.html
	jsWidth := js.Global().Get("contentWidth")
	jsHeight := js.Global().Get("contentHeight")
	screenWidth = jsWidth.Int()
	screenHeight = jsHeight.Int()

	fieldWidth = screenWidth
	fieldHeight = screenHeight - 64
	racketWidth = fieldWidth / 4
	fmt.Println("screen W&H: ", screenWidth, screenHeight)
	fmt.Println("field W&H: ", fieldWidth, fieldHeight)

	// Create the mirror floats, to alleviate the cpu
	fieldWidthF = float32(fieldWidth)
	fieldWidthFF = float64(fieldWidth)
	fieldHeightF = float32(fieldHeight)
	fieldHeightFF = float64(fieldHeight)
	racketWidthF = float32(racketWidth)
	racketHeightF = float32(racketHeight)

	// Initialize the game
	g.State = 0
	g.Score.Player = 0
	g.Score.CPU = 0

	g.Balls = []Ball{}

	g.Racket.X = fieldWidth/2 - racketWidth/2
	g.Racket.Y = fieldHeight - 1 - 5 - racketHeight
	g.Racket.Speed = racketSpeed

	// Initialize the images (arrows)
	arrowRight, _, _ = ebitenutil.NewImageFromFileSystem(embeddedFS, "res/icons8-arrow-64R.png")
	arrowLeft, _, _ = ebitenutil.NewImageFromFileSystem(embeddedFS, "res/icons8-arrow-64L.png")

	// Initialize fonts to show scoring
	f, _ := embeddedFS.Open("res/pressstart2p.ttf")
	fontFaceSource, _ = text.NewGoTextFaceSource(f)

	// Initalize audio (only if it starts, not when re-starting)
	if g.audioContext == nil {
		g.audioContext = audio.NewContext(48000)

		// Audio start
		fStart, err := embeddedFS.Open("res/game-start-6104.mp3")
		if err != nil {
			panic(err)
		}
		dStart, _ := mp3.DecodeWithoutResampling(fStart)
		g.audioPlayerStart, _ = g.audioContext.NewPlayer(dStart)

		// Audio hit the ball
		fHit, err := embeddedFS.Open("res/one_beep-99630.mp3")
		if err != nil {
			panic(err)
		}
		dHit, _ := mp3.DecodeWithoutResampling(fHit)
		g.audioPlayerHit, _ = g.audioContext.NewPlayer(dHit)

		// Audio misses the ball
		fMiss, err := embeddedFS.Open("res/coin-collect-retro-8-bit-sound-effect-145251.mp3")
		if err != nil {
			panic(err)
		}
		dMiss, _ := mp3.DecodeWithoutResampling(fMiss)
		g.audioPlayerMiss, _ = g.audioContext.NewPlayer(dMiss)

		// Audio game over
		fOver, err := embeddedFS.Open("res/cute-level-up-3-189853.mp3")
		if err != nil {
			panic(err)
		}
		dOver, _ := mp3.DecodeWithoutResampling(fOver)
		g.audioPlayerOver, _ = g.audioContext.NewPlayer(dOver)
	}

	// Start the match, take times
	deltaMatch = 0
	startMatch = time.Now()
}

func (g *Game) AddBall(rad int, R, G, B, A uint8, speedx, speedy int) {
	g.Balls = append(g.Balls, Ball{})
	last := len(g.Balls) - 1
	g.Balls[last].Radius = rad
	g.Balls[last].Color = color.RGBA{R, G, B, A}
	g.Balls[last].X = rand.IntN(fieldWidth)
	g.Balls[last].Y = rad
	g.Balls[last].SpeedX = speedx
	g.Balls[last].SpeedY = speedy
}

func (b *Ball) ResetBall() {
	b.X, b.Y = rand.IntN(fieldWidth), b.Radius
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (fieldWidth, fieldHeight int) {
	return outsideWidth, outsideHeight
}

func (g *Game) Update() error {
	// Pre-start the game, g.State = 0
	if g.State == 0 {
		// Tap to start the game
		touchIDs = inpututil.AppendJustReleasedTouchIDs(touchIDs[:0])
		if len(touchIDs) > 0 {
			g.State = 1
		}

		return nil
	}

	// Game over, g.State = 2
	if g.State == 2 {
		// Tap to restart the game, but wait 2s to lift your finger from the screen
		touchIDs = inpututil.AppendJustReleasedTouchIDs(touchIDs[:0])
		if len(touchIDs) > 0 && time.Now().Sub(startLiftFinger).Seconds() > 2 {
			g.Initialize()
			g.AddBall(5, 0, 255, 0, 0, 5, 5)
			g.State = 1
		}

		return nil
	}

	// Logic for the match, g.State = 1

	// Move the racket. You can use key arrows, mouse cursor or finger touch
	xCurs, yCurs := ebiten.CursorPosition()

	touchIDs = inpututil.AppendJustPressedTouchIDs(touchIDs[:0])
	for _, id := range touchIDs {
		xFinger, yFinger := ebiten.TouchPosition(id)
		fmt.Println("Pitjat: ", xFinger, yFinger)
		if xFinger > screenWidth-64 && yFinger > fieldHeight {
			touchRight = true
			touchLeft = false
			break
		}
		if xFinger < 64 && yFinger > fieldHeight {
			touchLeft = true
			touchRight = false
			break
		}

	}

	touchIDs = inpututil.AppendJustReleasedTouchIDs(touchIDs[:0])
	for _, id := range touchIDs {
		fmt.Println("Despitjat", id)
		touchRight, touchLeft = false, false
		break
	}

	if ebiten.IsKeyPressed(ebiten.KeyLeft) || (ebiten.IsMouseButtonPressed(ebiten.MouseButton0) && xCurs < 64 && yCurs > fieldHeight) || touchLeft {
		g.Racket.X -= g.Racket.Speed
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) || (ebiten.IsMouseButtonPressed(ebiten.MouseButton0) && xCurs > fieldWidth-64 && yCurs > fieldHeight) || touchRight {
		g.Racket.X += g.Racket.Speed
	}

	// Move the balls. Loop over each ball
	for i, _ := range g.Balls {
		// Move the ball b
		g.Balls[i].X += g.Balls[i].SpeedX
		g.Balls[i].Y += g.Balls[i].SpeedY

		// Ball out of the field lines, but not hitting racket or missing yet
		if (g.Balls[i].Y + 5) < g.Racket.Y {
			// Ball hits the walls
			if (g.Balls[i].X-g.Balls[i].Radius <= 0) || (g.Balls[i].X+g.Balls[i].Radius) >= (fieldWidth-1) {
				g.Balls[i].SpeedX = -g.Balls[i].SpeedX
			}

			// Ball hits the ceil
			if (g.Balls[i].Y - g.Balls[i].Radius) <= 0 {
				g.Balls[i].SpeedY = -g.Balls[i].SpeedY
			}

			continue
		}

		// Ball below the field line, hits or misses the racket
		// Hits the racket, else misses
		if g.Balls[i].X >= g.Racket.X && g.Balls[i].X <= (g.Racket.X+racketWidth) {
			//g.Balls[i].SpeedX = accelerate(g.Balls[i].SpeedX)
			//g.Balls[i].SpeedY = accelerateNRevers(g.Balls[i].SpeedY)
			g.Balls[i].SpeedY = -g.Balls[i].SpeedY
			g.Score.Player++
			go g.PlaySound(audioHit)
		} else {
			g.Balls[i].ResetBall()
			g.Score.CPU++
			go g.PlaySound(audioMiss)
		}
	}

	// Review scores and act accordingly:
	// -If reached winnerScore, game is over
	// -Add a new ball every 3 player points
	if g.Score.Player >= winnerScore || g.Score.CPU >= winnerScore {
		go g.PlaySound(audioOver)
		endMatch = time.Now()
		deltaMatch = endMatch.Sub(startMatch).Seconds()
		startLiftFinger = time.Now()
		g.State = 2
	} else if g.Score.Player == len(g.Balls)*3 {
		// Add a total random ball
		speed := randBetween(3, 6)
		g.AddBall(randBetween(4, 10), uint8(randBetween(0, 255)), uint8(randBetween(0, 255)), uint8(randBetween(0, 255)), 0, speed, speed)
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	// Draw the bottom line of the field
	vector.DrawFilledRect(screen, 0, fieldHeightF, fieldWidthF, 3, color.White, false)

	// Draw the arrows to move the racket
	opAr := &ebiten.DrawImageOptions{}
	opAr.GeoM.Translate(fieldWidthFF-64, fieldHeightFF+3)
	screen.DrawImage(arrowRight, opAr)

	opAr = &ebiten.DrawImageOptions{}
	opAr.GeoM.Translate(0, fieldHeightFF+3)
	screen.DrawImage(arrowLeft, opAr)

	// Draw initial screen, pre-start
	if g.State == 0 {
		vector.DrawFilledCircle(screen, fieldWidthF/2, fieldHeightF/2, 5, color.RGBA{R: 0, G: 255, B: 0, A: 0}, true)
		vector.DrawFilledRect(screen, float32(g.Racket.X), fieldHeightF-racketHeightF-5, racketWidthF, racketHeightF, color.White, false)
		ebitenutil.DebugPrint(screen, fmt.Sprintf("Tap the screen to play.\nMove racket with the arrows.\nFirst to score %d wins. Enjoy!", winnerScore))

		opSt := &text.DrawOptions{}
		opSt.GeoM.Translate(fieldWidthFF/2-20*5/2, fieldHeightFF+20)
		opSt.ColorScale.ScaleWithColor(color.White)
		text.Draw(screen, "00:00", &text.GoTextFace{
			Source: fontFaceSource,
			Size:   20,
		}, opSt)

		return
	}

	// Draw game over
	if g.State == 2 {
		if g.Score.Player >= winnerScore {
			ebitenutil.DebugPrint(screen, fmt.Sprintf("You won.\nMatch duration %.2fs.\nTap the screen to play again.", deltaMatch))
		} else {
			ebitenutil.DebugPrint(screen, fmt.Sprintf("Machine won.\nMatch duration %.2fs.\nTap the screen to play again.", deltaMatch))
		}

		opGO := &text.DrawOptions{}
		opGO.GeoM.Translate(fieldWidthFF/2-20*9/2, fieldHeightFF/3)
		opGO.ColorScale.ScaleWithColor(color.White)
		text.Draw(screen, "GAME OVER", &text.GoTextFace{
			Source: fontFaceSource,
			Size:   20,
		}, opGO)

		opGO = &text.DrawOptions{}
		opGO.GeoM.Translate(fieldWidthFF/2-20*5/2, fieldHeightFF+20)
		opGO.ColorScale.ScaleWithColor(color.White)
		text.Draw(screen, fmt.Sprintf("%02d:%02d", g.Score.Player, g.Score.CPU), &text.GoTextFace{
			Source: fontFaceSource,
			Size:   20,
		}, opGO)

		return
	}

	// Draw the match

	// Draw balls
	for _, b := range g.Balls {
		vector.DrawFilledCircle(screen, float32(b.X), float32(b.Y), float32(b.Radius), b.Color, true)
	}
	// Draw racket
	vector.DrawFilledRect(screen, float32(g.Racket.X), fieldHeightF-racketHeightF-5, racketWidthF, racketHeightF, color.White, false)

	// Print score
	opSc := &text.DrawOptions{}
	opSc.GeoM.Translate(fieldWidthFF/2-20*5/2, fieldHeightFF+20)
	opSc.ColorScale.ScaleWithColor(color.White)
	text.Draw(screen, fmt.Sprintf("%02d:%02d", g.Score.Player, g.Score.CPU), &text.GoTextFace{
		Source: fontFaceSource,
		Size:   20,
	}, opSc)
}

// Return a random integer between n and m, both included
func randBetween(n, m int) int {
	if !(m > n) {
		return 0
	}
	return rand.IntN(m+1-n) + n
}

// Play a sound for each situation (sit)
func (g *Game) PlaySound(sit int) {
	switch sit {
	case audioStart:
		g.audioPlayerStart.Rewind()
		g.audioPlayerStart.Play()
	case audioHit:
		g.audioPlayerHit.Rewind()
		g.audioPlayerHit.Play()
	case audioMiss:
		g.audioPlayerMiss.Rewind()
		g.audioPlayerMiss.Play()
	case audioOver:
		g.audioPlayerOver.Rewind()
		g.audioPlayerOver.Play()
	}
}

func main() {
	g := Game{}
	g.Initialize()
	g.AddBall(5, 0, 255, 0, 0, 5, 5)

	ebiten.SetWindowSize(screenWidth, screenHeight)

	err := ebiten.RunGame(&g)
	if err != nil {
		panic(err)
	}
}
