package handlers

import (
	"context"
	"errors"
	"strconv"
	"sync"

	"seanime/internal/api/anilist"
	"seanime/internal/library/anime"
	"seanime/internal/stremio"

	"github.com/labstack/echo/v4"
)

// media-player: the Movies & TV section (internal/stremio).

var (
	stremioOnce    sync.Once
	stremioService *stremio.Service
)

type stremioPlayer struct{ h *Handler }

func (p stremioPlayer) PlayMediaUrlStream(ctx context.Context, streamURL, clientID string, media *anilist.BaseAnime, episode *anime.Episode) error {
	if p.h.App.DirectStreamManager == nil {
		return errors.New("the player is not ready yet")
	}
	return p.h.App.DirectStreamManager.PlayMediaUrlStream(ctx, streamURL, clientID, media, episode)
}

func (h *Handler) stremio() *stremio.Service {
	stremioOnce.Do(func() {
		client := stremio.NewClient(func() string {
			s, ok := h.App.Database.GetDebridSettings()
			if !ok || s == nil || !s.Enabled || s.Provider != "realdebrid" {
				return ""
			}
			return s.ApiKey
		})
		stremioService = &stremio.Service{
			Client:     client,
			Library:    stremio.NewLibrary(h.App.Config.Data.AppDataDir),
			Player:     stremioPlayer{h},
			Continuity: h.App.ContinuityManager,
			Logger:     h.App.Logger,
		}
	})
	return stremioService
}

// HandleStremioCatalog
//
//	@summary returns a Cinemeta catalog (top, imdbRating, year) of movies or series.
//	@route /api/v1/stremio/catalog/{type}/{id} [GET]
func (h *Handler) HandleStremioCatalog(c echo.Context) error {
	skip, _ := strconv.Atoi(c.QueryParam("skip"))
	metas, err := h.stremio().Client.Catalog(c.Request().Context(), c.Param("type"), c.Param("id"), skip, c.QueryParam("genre"))
	if err != nil {
		return h.RespondWithError(c, err)
	}
	return h.RespondWithData(c, metas)
}

// HandleStremioSearch
//
//	@summary searches movies and series.
//	@route /api/v1/stremio/search [GET]
func (h *Handler) HandleStremioSearch(c echo.Context) error {
	metas, err := h.stremio().Client.Search(c.Request().Context(), c.QueryParam("q"))
	if err != nil {
		return h.RespondWithError(c, err)
	}
	return h.RespondWithData(c, metas)
}

// HandleStremioMeta
//
//	@summary returns a title's details and, for a series, its episodes.
//	@route /api/v1/stremio/meta/{type}/{id} [GET]
func (h *Handler) HandleStremioMeta(c echo.Context) error {
	meta, err := h.stremio().Client.Meta(c.Request().Context(), c.Param("type"), c.Param("id"))
	if err != nil {
		return h.RespondWithError(c, err)
	}
	return h.RespondWithData(c, meta)
}

// HandleStremioStreams
//
//	@summary returns Real-Debrid streams from Torrentio (and Comet as a backup).
//	@route /api/v1/stremio/streams/{type}/{id} [GET]
func (h *Handler) HandleStremioStreams(c echo.Context) error {
	streams, err := h.stremio().Client.Streams(c.Request().Context(), c.Param("type"), c.Param("id"))
	if err != nil {
		return h.RespondWithError(c, err)
	}
	return h.RespondWithData(c, streams)
}

// HandleStremioPlay
//
//	@summary plays a stream in the built-in player.
//	@route /api/v1/stremio/play [POST]
func (h *Handler) HandleStremioPlay(c echo.Context) error {
	var b stremio.PlayRequest
	if err := c.Bind(&b); err != nil {
		return h.RespondWithError(c, err)
	}
	b.ClientID = getRequestClientId(c, b.ClientID)
	if err := h.stremio().Play(context.Background(), b); err != nil {
		return h.RespondWithError(c, err)
	}
	return h.RespondWithData(c, true)
}

// HandleStremioPlayNext
//
//	@summary plays the next episode of the series last played under mediaId.
//	@route /api/v1/stremio/play-next [POST]
func (h *Handler) HandleStremioPlayNext(c echo.Context) error {
	var b struct {
		MediaID  int    `json:"mediaId"`
		ClientID string `json:"clientId"`
	}
	if err := c.Bind(&b); err != nil {
		return h.RespondWithError(c, err)
	}
	if err := h.stremio().PlayNext(context.Background(), b.MediaID, getRequestClientId(c, b.ClientID)); err != nil {
		return h.RespondWithError(c, err)
	}
	return h.RespondWithData(c, true)
}

// HandleStremioContinue
//
//	@summary returns Continue Watching and Next Up for movies and series.
//	@route /api/v1/stremio/continue [GET]
func (h *Handler) HandleStremioContinue(c echo.Context) error {
	return h.RespondWithData(c, h.stremio().Continue(c.Request().Context()))
}

// HandleStremioForget
//
//	@summary removes a title from Continue Watching.
//	@route /api/v1/stremio/forget [POST]
func (h *Handler) HandleStremioForget(c echo.Context) error {
	var b struct {
		MediaID int `json:"mediaId"`
	}
	if err := c.Bind(&b); err != nil || b.MediaID >= 0 {
		return h.RespondWithError(c, errors.New("invalid media id"))
	}
	id := b.MediaID
	h.stremio().Library.Forget(id)
	_ = h.App.ContinuityManager.DeleteWatchHistoryItem(id)
	return h.RespondWithData(c, true)
}
