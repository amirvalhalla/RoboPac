package discord

import (
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/kehiy/RoboPac/engine"
	"github.com/kehiy/RoboPac/log"
	"github.com/kehiy/RoboPac/utils"
)

type DiscordBot struct {
	Session   *discordgo.Session
	BotEngine *engine.BotEngine
	GuildID   string
}

func NewDiscordBot(botEngine *engine.BotEngine, token, guildID string) (*DiscordBot, error) {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}

	return &DiscordBot{
		Session:   s,
		BotEngine: botEngine,
		GuildID:   guildID,
	}, nil
}

func (bot *DiscordBot) Start() error {
	log.Info("starting Discord Bot...")

	err := bot.Session.Open()
	if err != nil {
		return err
	}

	bot.deleteAllCommands()
	return bot.registerCommands()
}

func (bot *DiscordBot) deleteAllCommands() {
	cmdsServer, _ := bot.Session.ApplicationCommands(bot.Session.State.User.ID, bot.GuildID)
	cmdsGlobal, _ := bot.Session.ApplicationCommands(bot.Session.State.User.ID, "")
	cmds := append(cmdsServer, cmdsGlobal...) //nolint

	for _, cmd := range cmds {
		err := bot.Session.ApplicationCommandDelete(cmd.ApplicationID, cmd.GuildID, cmd.ID)
		if err != nil {
			log.Error("unable to delete command", "error", err, "cmd", cmd.Name)
		} else {
			log.Info("discord command unregistered", "name", cmd.Name)
		}
	}
}

func (bot *DiscordBot) registerCommands() error {
	bot.Session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		bot.commandHandler(bot, s, i)
	})

	beCmds := bot.BotEngine.Commands()
	for _, beCmd := range beCmds {
		if !beCmd.HasAppId(engine.AppIdDiscord) {
			continue
		}
		discordCmd := discordgo.ApplicationCommand{
			Name:        beCmd.Name,
			Description: beCmd.Desc,
			Options:     make([]*discordgo.ApplicationCommandOption, len(beCmd.Args)),
		}
		for index, arg := range beCmd.Args {
			discordCmd.Options[index] = &discordgo.ApplicationCommandOption{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        arg.Name,
				Description: arg.Desc,
				Required:    !arg.Optional,
			}
		}

		cmd, err := bot.Session.ApplicationCommandCreate(bot.Session.State.User.ID, "", &discordCmd)
		if err != nil {
			log.Error("can not register discord command", "name", discordCmd.Name, "error", err)
			return err
		}
		log.Info("discord command registered", "name", cmd.Name)
	}

	return nil
}

func (bot *DiscordBot) commandHandler(db *DiscordBot, s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.GuildID != "" {
		bot.respondErrMsg("Send a message in a bottle, ye say? Cast it into me DMs, and I'll be at yer service!", s, i)
		return
	}

	beInput := []string{}

	// Get the application command data
	discordCmd := i.ApplicationCommandData()
	beInput = append(beInput, discordCmd.Name)
	for _, opt := range discordCmd.Options {
		beInput = append(beInput, opt.StringValue())
	}

	res, err := db.BotEngine.Run(engine.AppIdDiscord, i.User.ID, beInput)
	if err != nil {
		db.respondErrMsg(err.Error(), s, i)
		return
	}

	bot.respondResultMsg(res, s, i)
}

func (bot *DiscordBot) respondErrMsg(errStr string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	errorEmbed := &discordgo.MessageEmbed{
		Title:       "Error",
		Description: errStr,
		Color:       RED,
	}
	bot.respondEmbed(errorEmbed, s, i)
}

func (bot *DiscordBot) respondResultMsg(res *engine.CommandResult, s *discordgo.Session, i *discordgo.InteractionCreate) {
	var resEmbed *discordgo.MessageEmbed
	if res.Successful {
		resEmbed = &discordgo.MessageEmbed{
			Title:       "Successful",
			Description: res.Message,
			Color:       GREEN,
		}
	} else {
		resEmbed = &discordgo.MessageEmbed{
			Title:       "Failed",
			Description: res.Message,
			Color:       YELLOW,
		}
	}

	bot.respondEmbed(resEmbed, s, i)
}

func (db *DiscordBot) respondEmbed(embed *discordgo.MessageEmbed, s *discordgo.Session, i *discordgo.InteractionCreate) {
	response := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	}

	err := s.InteractionRespond(i.Interaction, response)
	if err != nil {
		log.Error("InteractionRespond error:", "error", err)
	}
}

func (db *DiscordBot) UpdateStatusInfo() {
	log.Info("info status started")
	for {
		ns, err := db.BotEngine.NetworkStatus()
		if err != nil {
			continue
		}

		err = db.Session.UpdateStatusComplex(newStatus("validators count", utils.FormatNumber(int64(ns.ValidatorsCount))))
		if err != nil {
			log.Error("can't set status", "err", err)
			continue
		}

		time.Sleep(time.Second * 5)

		err = db.Session.UpdateStatusComplex(newStatus("total accounts", utils.FormatNumber(int64(ns.TotalAccounts))))
		if err != nil {
			log.Error("can't set status", "err", err)
			continue
		}

		time.Sleep(time.Second * 5)

		err = db.Session.UpdateStatusComplex(newStatus("height", utils.FormatNumber(int64(ns.CurrentBlockHeight))))
		if err != nil {
			log.Error("can't set status", "err", err)
			continue
		}

		time.Sleep(time.Second * 5)

		err = db.Session.UpdateStatusComplex(newStatus("circ supply",
			utils.FormatNumber(int64(utils.ChangeToCoin(ns.CirculatingSupply)))+" PAC"))
		if err != nil {
			log.Error("can't set status", "err", err)
			continue
		}

		time.Sleep(time.Second * 5)

		err = db.Session.UpdateStatusComplex(newStatus("total power",
			utils.FormatNumber(int64(utils.ChangeToCoin(ns.TotalNetworkPower)))+" PAC"))
		if err != nil {
			log.Error("can't set status", "err", err)
			continue
		}

		time.Sleep(time.Second * 5)
	}
}

func (db *DiscordBot) Stop() {
	log.Info("shutting down Discord Bot...")

	_ = db.Session.Close()
}
