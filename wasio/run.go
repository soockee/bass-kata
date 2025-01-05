package wasio

import (
	"log"
	"log/slog"

	"github.com/JamesDunne/go-asio"
)

func Run() {

	driver, err := asio.ListDrivers()
	if err != nil {
		log.Fatal(err)
	}
	for _, d := range driver {
		slog.Debug("Driver", slog.Any("driver", d.Name))
	}
}
