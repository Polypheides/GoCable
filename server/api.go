package server

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/Polypheides/go-homelab-cable/domain"
	"github.com/Polypheides/go-homelab-cable/network"
	"github.com/labstack/echo/v4"
)

func (s *Server) getNetworks(e echo.Context) error {
	host := e.Request().Host
	if host == "" || host == "localhost" || host == "127.0.0.1" || strings.HasPrefix(host, "127.0.0.1:") || strings.HasPrefix(host, "localhost:") {
		host = network.GetLocalIP() + ":" + s.Network.WebServerPort
	}
	return e.JSON(http.StatusOK, []domain.Network{
		{
			Name:                s.Network.Name,
			Owner:               s.Network.Owner,
			CallSign:            s.Network.CallSign,
			MasterStreamURL:     s.Network.MasterStreamURL(),
			HttpMasterStreamURL: fmt.Sprintf("http://%s/master", host),
		},
	})
}

func (s *Server) getChannels(e echo.Context) error {
	channels := s.Network.Channels()
	models := make([]domain.Channel, 0, len(channels))
	host := e.Request().Host
	for _, c := range channels {
		models = append(models, domain.ToChannelModel(s.Network, c, host))
	}

	// Sort by StreamURL (Port) to keep the CLI stable
	sort.Slice(models, func(i, j int) bool {
		return models[i].StreamURL < models[j].StreamURL
	})

	return e.JSON(http.StatusOK, models)
}

func (s *Server) getChannel(e echo.Context) error {
	c, err := s.Network.Channel(e.Param("channel_id"))
	if err != nil {
		if errors.Is(err, network.ErrNetworkChannelNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return err
	}
	return s.jsonChannel(e, c)
}

func (s *Server) setChannelLive(e echo.Context) error {
	idParam := e.Param("channel_id")

	var c *network.Channel
	var err error

	// Try to parse the param as a friendly channel number first
	if num, parseErr := strconv.Atoi(idParam); parseErr == nil {
		c, err = s.Network.ChannelByNumber(num)
	} else {
		// Fallback to UUID lookup
		c, err = s.Network.Channel(idParam)
	}

	if err != nil {
		if errors.Is(err, network.ErrNetworkChannelNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return err
	}

	err = s.Network.SetChannelLive(c.ID)
	if err != nil {
		return err
	}
	s.logAction("TUNE", e.Request().URL.Path, c)
	return s.jsonChannel(e, c)
}

func (s *Server) playNext(e echo.Context) error {
	c, err := s.Network.Channel(e.Param("channel_id"))
	if err != nil {
		if errors.Is(err, network.ErrNetworkChannelNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return err
	}
	_ = c.PlayNext()
	s.logAction("PUT", e.Request().URL.Path, c)
	return s.jsonChannel(e, c)
}

func (s *Server) playLiveNext(e echo.Context) error {
	c, err := s.Network.CurrentChannel()
	if err != nil {
		if errors.Is(err, network.ErrNetworkNoChannelPlaying) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return err
	}
	_ = c.PlayNext()
	s.logAction("PUT", e.Request().URL.Path, c)
	return s.jsonChannel(e, c)
}

func (s *Server) liveChannel(e echo.Context) error {
	c, err := s.Network.CurrentChannel()
	if err != nil {
		if errors.Is(err, network.ErrNetworkNoChannelPlaying) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return err
	}
	return s.jsonChannel(e, c)
}

func (s *Server) jsonChannel(e echo.Context, c *network.Channel) error {
	return e.JSON(http.StatusOK, domain.ToChannelModel(s.Network, c, e.Request().Host))
}
