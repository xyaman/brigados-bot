package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"

	"strings"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/session"
	"github.com/diamondburned/arikawa/v3/voice"
	"github.com/diamondburned/oggreader"
)

type Player struct {
	voiceSession *voice.Session
	channel      chan Track
}

type Track struct {
	title         string
	url           string
	requester     string
	textChannelID string
}

func (p *Player) appendTrack(track Track) {
	p.channel <- track
}

func (p *Player) mainLoop() {
	for {
		track := <-p.channel

		cmd := fmt.Sprintf("yt-dlp --quiet --ignore-errors --flat-playlist -o - '%s' | ffmpeg -i pipe:0 -f opus pipe:1", track.url)
		ytdlp := exec.Command("bash", "-c", cmd)

		stdout, err := ytdlp.StdoutPipe()
		if err != nil {
			fmt.Println("Cant open pipe, Error:", err)
			return
		}

		err = ytdlp.Start()
		if err != nil {
			fmt.Println("Cant open ytdl, Error:", err)
			return
		}

		if err := oggreader.DecodeBuffered(p.voiceSession, stdout); err != nil {
			_ = fmt.Errorf("failed to decode ogg: %w", err)
			return
		}
		ytdlp.Wait()
	}
}

func main() {
	// guildid := "795683075212312636"
	var channelid discord.ChannelID = 1350659882730258565
	token := ""

	player := &Player{
		channel: make(chan Track),
	}

	s := session.New("Bot " + token)
	s.AddHandler(func(c *gateway.ReadyEvent) {
		fmt.Printf("%s is ready.\n", c.User.Username)

		v, err := voice.NewSession(s)
		if err != nil {
			log.Fatalln("Can't create voice session")
		}

		err = v.JoinChannelAndSpeak(context.TODO(), channelid, false, true)
		if err != nil {
			fmt.Println("Can't join create voice channel")
			return
		}

		player.voiceSession = v

		go player.mainLoop()
	})

	s.AddHandler(func(c *gateway.MessageCreateEvent) {
		// si el mensaje comeinza con !play url [1]
		message := strings.TrimSpace(c.Content)
		fmt.Println("Mensaje: ", message)

		if !strings.HasPrefix(message, "!play ") {
			return
		}

		args := strings.Split(message, " ")
		url := args[1]

		track := Track{
			title:     "",
			url:       url,
			requester: c.Author.Username,
		}

		// Invoke audio player
		player.appendTrack(track)
	})

	voice.AddIntents(s)
	s.AddIntents(gateway.IntentDirectMessages)
	s.AddIntents(gateway.IntentGuildMessages)

	// connect to discord
	// TODO: see connect method
	if err := s.Connect(context.Background()); err != nil {
		log.Fatal("Can't connect to discord:", err)
	}
}
