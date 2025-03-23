package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"sync"
	"syscall"

	"strings"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/diamondburned/arikawa/v3/voice"
)

var ActivePlayers = make(map[discord.GuildID]*Player)
var playersMutex = sync.Mutex{}

type Player struct {
	voiceSession *voice.Session
	isPlaying    bool
	channel      chan Track
	stopCh       chan struct{}
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
		p.isPlaying = true

		cmd := fmt.Sprintf("yt-dlp --quiet --ignore-errors --flat-playlist -o - '%s' | ffmpeg -i pipe:0 -f opus pipe:1", track.url)
		ytdlp := exec.Command("bash", "-c", cmd)
		ytdlp.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		stdout, err := ytdlp.StdoutPipe()
		if err != nil {
			fmt.Println("Cant open pipe, Error:", err)
			return
		}

		err = ytdlp.Start()
		log.Println("Starting new ytdl process. Url:", track.url)
		if err != nil {
			fmt.Println("Cant open ytdl, Error:", err)
			return
		}
 
		if err := DecodeBuffered(p.voiceSession, stdout, p.stopCh); err != nil {
			// _ = fmt.Errorf("failed to decode ogg: %w", err)

			// kill main process (bash)
			ytdlp.Process.Kill()

			// kill spawned children (ytdlp & bash)
			syscall.Kill(-ytdlp.Process.Pid, syscall.SIGKILL)
			p.isPlaying = false;
			continue
		}

		ytdlp.Wait()
		p.isPlaying = false;
	}
}

func main() {
	token := ""

	s := state.New("Bot " + token)
	s.AddHandler(func(c *gateway.ReadyEvent) {
		log.Printf("%s is ready.\n", c.User.Username)
	})

	s.AddHandler(func(c *gateway.MessageCreateEvent) {
		msg := strings.TrimSpace(c.Content)

		if msg == "!stop" {
			player, ok := ActivePlayers[c.GuildID]
			if ok {
				// TODO: send exit signal to cancel current process ffmpeg etc
				player.voiceSession.Leave(context.TODO())
				close(player.stopCh)
				delete(ActivePlayers, c.GuildID)
			} else {
				s.SendMessageReply(c.ChannelID, "I'm not in any channel", c.ID)
			}

			return
		}

		if msg == "!skip" {
			player, ok := ActivePlayers[c.GuildID]
			if ok && player.isPlaying {
				player.stopCh <- struct{}{}
			} else {
				s.SendMessageReply(c.ChannelID, "I'm not in any channel", c.ID)
			}

			return
		}

		if !strings.HasPrefix(msg, "!play ") {
			return
		}
		_, ok := ActivePlayers[c.GuildID]
		if !ok {
			vs, err := s.VoiceState(c.GuildID, c.Member.User.ID)
			if err != nil {
				s.SendMessageReply(c.ChannelID, "You need to be in a voice channel", c.ID)
				return
			}

			v, err := voice.NewSession(s)
			err = v.JoinChannelAndSpeak(context.TODO(), vs.ChannelID, false, true)
			if err != nil {
				fmt.Println("Can't join create voice channel")
				return
			}

			player := &Player{
				channel:      make(chan Track),
				voiceSession: v,
				stopCh:       make(chan struct{}),
			}

			ActivePlayers[c.GuildID] = player
			go ActivePlayers[c.GuildID].mainLoop()
		}

		args := strings.Split(msg, " ")
		url := args[1]

		track := Track{
			title:     "",
			url:       url,
			requester: c.Author.Username,
		}

		ActivePlayers[c.GuildID].appendTrack(track)
	})

	voice.AddIntents(s)
	s.AddIntents(gateway.IntentDirectMessages)
	s.AddIntents(gateway.IntentGuildMessages)
	s.AddIntents(gateway.IntentGuildVoiceStates)

	if err := s.Connect(context.Background()); err != nil {
		log.Fatal("Can't connect to discord:", err)
	}
}

func fetchTrackMetadata() {
}
