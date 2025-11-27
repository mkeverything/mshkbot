// cmd/main.go
package main

import (
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/sukalov/mshkbot/internal/bot"
	"github.com/sukalov/mshkbot/internal/cron"
	"github.com/sukalov/mshkbot/internal/db"
	"github.com/sukalov/mshkbot/internal/handlers/admingroup"
	"github.com/sukalov/mshkbot/internal/handlers/maingroup"
	"github.com/sukalov/mshkbot/internal/handlers/privatechat"
	"github.com/sukalov/mshkbot/internal/utils"
)

func main() {
	// load environment variables
	env, err := utils.LoadEnv([]string{
		"BOT_TOKEN",
		"MAIN_GROUP_ID",
		"ADMIN_GROUP_ID",
	})
	if err != nil {
		log.Fatalf("failed to load env: %v", err)
	}

	// parse group ids
	mainGroupID, err := strconv.ParseInt(env["MAIN_GROUP_ID"], 10, 64)
	if err != nil {
		log.Fatalf("invalid MAIN_GROUP_ID: %v", err)
	}

	adminGroupID, err := strconv.ParseInt(env["ADMIN_GROUP_ID"], 10, 64)
	if err != nil {
		log.Fatalf("invalid ADMIN_GROUP_ID: %v", err)
	}

	// create bot instance
	botInstance, err := bot.New("mshkbot", env["BOT_TOKEN"], mainGroupID, adminGroupID)
	if err != nil {
		log.Fatalf("failed to create bot: %v", err)
	}

	// create scheduler
	scheduler := cron.New(botInstance, mainGroupID, adminGroupID)

	// get handlers from each package
	mainGroupHandlers := maingroup.GetHandlers()
	adminGroupHandlers := admingroup.GetHandlers(scheduler)
	privateHandlers := privatechat.GetHandlers()

	// start bot and scheduler in goroutines
	go botInstance.Start(mainGroupHandlers, adminGroupHandlers, privateHandlers)
	go scheduler.Start()

	// wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	// cleanup
	log.Println("shutting down...")
	scheduler.Stop()
	botInstance.Stop()
	db.Close()
	log.Println("shutdown complete")
}
