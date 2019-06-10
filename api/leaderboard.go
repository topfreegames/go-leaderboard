// podium
// https://github.com/topfreegames/podium
// Licensed under the MIT license:
// http://www.opensource.org/licenses/mit-license
// Copyright © 2016 Top Free Games <backend@tfgco.com>
// Forked from
// https://github.com/dayvson/go-leaderboard
// Copyright © 2013 Maxwell Dayvson da Silva

package api

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/buger/jsonparser"
	"github.com/labstack/echo"
	"github.com/topfreegames/podium/leaderboard"
	"go.uber.org/zap"
)

var notFoundError = "Could not find data for member"
var noPageSizeProvidedError = "strconv.ParseInt: parsing \"\": invalid syntax"
var defaultPageSize = 20

func serializeMember(member *leaderboard.Member, position int, includeTTL bool) map[string]interface{} {
	memberData := map[string]interface{}{
		"publicID": member.PublicID,
		"score":    member.Score,
		"rank":     member.Rank,
	}
	if member.PreviousRank != 0 {
		memberData["previousRank"] = member.PreviousRank
	}
	if position >= 0 {
		memberData["position"] = position
	}
	if includeTTL {
		memberData["expireAt"] = member.ExpireAt
	}
	return memberData
}

func serializeMembers(members leaderboard.Members, includePosition bool, includeTTL bool) []map[string]interface{} {
	serializedMembers := make([]map[string]interface{}, len(members))
	for i, member := range members {
		if includePosition {
			serializedMembers[i] = serializeMember(member, i, includeTTL)
		} else {
			serializedMembers[i] = serializeMember(member, -1, includeTTL)
		}
	}
	return serializedMembers
}

// BulkUpsertMembersScoreHandler is the handler responsible for creating or updating members score
func BulkUpsertMembersScoreHandler(app *App) func(c echo.Context) error {
	return func(c echo.Context) error {
		leaderboardID := c.Param("leaderboardID")
		lg := app.Logger.With(
			zap.String("handler", "BulkUpsertMembersScoreHandler"),
			zap.String("leaderboard", leaderboardID),
		)

		var payload setMembersScorePayload
		prevRank := c.QueryParam("prevRank") == "true"
		scoreTTL := c.QueryParam("scoreTTL")

		err := WithSegment("Payload", c, func() error {
			if err := LoadJSONPayload(&payload, c, lg); err != nil {
				app.AddError()
				return err
			}
			return nil
		})
		if err != nil {
			return FailWith(400, err.Error(), c)
		}

		members := make(leaderboard.Members, len(payload.MembersScore))
		err = WithSegment("Model", c, func() error {
			lg.Debug("Setting member scores.")
			l := leaderboard.NewLeaderboard(app.RedisClient.Trace(c.StdContext()), leaderboardID, 0)
			for i, ms := range payload.MembersScore {
				members[i] = &leaderboard.Member{Score: ms.Score, PublicID: ms.PublicID}
			}
			err = l.SetMembersScore(members, prevRank, scoreTTL)

			if err != nil {
				lg.Error("Setting member scores failed.", zap.Error(err))
				app.AddError()
				return err
			}
			lg.Debug("Setting member scores succeeded.")
			return nil
		})
		if err != nil {
			return FailWithError(err, c)
		}
		return SucceedWith(map[string]interface{}{
			"members": serializeMembers(members, false, scoreTTL != ""),
		}, c)
	}
}

// UpsertMemberScoreHandler is the handler responsible for creating or updating the member score
func UpsertMemberScoreHandler(app *App) func(c echo.Context) error {
	return func(c echo.Context) error {
		leaderboardID := c.Param("leaderboardID")
		memberPublicID := c.Param("memberPublicID")
		lg := app.Logger.With(
			zap.String("handler", "UpsertMemberScoreHandler"),
			zap.String("leaderboard", leaderboardID),
			zap.String("memberPublicID", memberPublicID),
		)

		var payload setScorePayload
		prevRank := false
		prevRankStr := c.QueryParam("prevRank")
		if prevRankStr != "" && prevRankStr == "true" {
			prevRank = true
		}
		scoreTTL := c.QueryParam("scoreTTL")

		err := WithSegment("Payload", c, func() error {
			b, err := GetRequestBody(c)
			if err != nil {
				app.AddError()
				return err
			}
			if _, err := jsonparser.GetInt(b, "score"); err != nil {
				app.AddError()
				if _, t, _, err := jsonparser.Get(b, "score"); err == nil {
					return fmt.Errorf("invalid type for score: %v", t)
				}
				return fmt.Errorf("score is required")
			}
			if err := LoadJSONPayload(&payload, c, lg); err != nil {
				app.AddError()
				return err
			}
			return nil
		})
		if err != nil {
			return FailWith(400, err.Error(), c)
		}

		var member *leaderboard.Member
		err = WithSegment("Model", c, func() error {
			lg.Debug("Setting member score.", zap.Int64("score", payload.Score))
			l := leaderboard.NewLeaderboard(app.RedisClient.Trace(c.StdContext()), leaderboardID, 0)
			member, err = l.SetMemberScore(memberPublicID, payload.Score, prevRank, scoreTTL)

			if err != nil {
				lg.Error("Setting member score failed.", zap.Error(err))
				app.AddError()
				return err
			}
			lg.Debug("Setting member score succeeded.")
			return nil
		})
		if err != nil {
			return FailWithError(err, c)
		}
		return SucceedWith(serializeMember(member, -1, scoreTTL != ""), c)
	}
}

// IncrementMemberScoreHandler is the handler responsible for incrementing the member score
func IncrementMemberScoreHandler(app *App) func(c echo.Context) error {
	return func(c echo.Context) error {
		leaderboardID := c.Param("leaderboardID")
		memberPublicID := c.Param("memberPublicID")
		scoreTTL := c.QueryParam("scoreTTL")
		lg := app.Logger.With(
			zap.String("handler", "IncrementMemberScoreHandler"),
			zap.String("leaderboard", leaderboardID),
			zap.String("memberPublicID", memberPublicID),
		)

		var payload incrementScorePayload

		err := WithSegment("Payload", c, func() error {
			if err := LoadJSONPayload(&payload, c, lg); err != nil {
				app.AddError()
				return err
			}
			return nil
		})
		if err != nil {
			return FailWith(400, err.Error(), c)
		}

		var member *leaderboard.Member
		err = WithSegment("Model", c, func() error {
			lg.Debug("Incrementing member score.", zap.Int("increment", payload.Increment))
			l := leaderboard.NewLeaderboard(app.RedisClient.Trace(c.StdContext()), leaderboardID, 0)
			member, err = l.IncrementMemberScore(memberPublicID, payload.Increment, scoreTTL)

			if err != nil {
				lg.Error("Member score increment failed.", zap.Error(err))
				app.AddError()
				return err
			}
			lg.Debug("Member score increment succeeded.")
			return nil
		})
		if err != nil {
			return FailWithError(err, c)
		}

		return SucceedWith(serializeMember(member, -1, scoreTTL != ""), c)
	}
}

//RemoveMemberHandler removes a member from a leaderboard
func RemoveMemberHandler(app *App) func(c echo.Context) error {
	return func(c echo.Context) error {
		leaderboardID := c.Param("leaderboardID")
		memberPublicID := c.Param("memberPublicID")
		lg := app.Logger.With(
			zap.String("handler", "RemoveMemberHandler"),
			zap.String("leaderboard", leaderboardID),
			zap.String("memberPublicID", memberPublicID),
		)

		err := WithSegment("Model", c, func() error {
			lg.Debug("Removing member.")
			l := leaderboard.NewLeaderboard(app.RedisClient.Trace(c.StdContext()), leaderboardID, 0)
			err := l.RemoveMember(memberPublicID)

			if err != nil && !strings.HasPrefix(err.Error(), notFoundError) {
				lg.Error("Member removal failed.", zap.Error(err))
				app.AddError()
				return err
			}
			lg.Debug("Member removal succeeded.")
			return nil
		})
		if err != nil {
			return FailWith(500, err.Error(), c)
		}

		return SucceedWith(map[string]interface{}{}, c)
	}
}

//RemoveMembersHandler removes several members from a leaderboard
func RemoveMembersHandler(app *App) func(c echo.Context) error {
	return func(c echo.Context) error {
		leaderboardID := c.Param("leaderboardID")
		lg := app.Logger.With(
			zap.String("handler", "RemoveMembersHandler"),
			zap.String("leaderboard", leaderboardID),
		)

		ids := c.QueryParam("ids")
		if ids == "" {
			app.AddError()
			return FailWith(400, "Member IDs are required using the 'ids' querystring parameter", c)
		}

		memberIDs := strings.Split(ids, ",")
		idsInter := make([]interface{}, len(memberIDs))
		for i, v := range memberIDs {
			idsInter[i] = v
		}

		err := WithSegment("Model", c, func() error {
			lg.Debug("Removing members.", zap.String("ids", ids))
			l := leaderboard.NewLeaderboard(app.RedisClient.Trace(c.StdContext()), leaderboardID, 0)
			err := l.RemoveMembers(idsInter)

			if err != nil && !strings.HasPrefix(err.Error(), notFoundError) {
				lg.Error("Members removal failed.", zap.Error(err))
				app.AddError()
				return err
			}
			lg.Debug("Members removal succeeded.")
			return nil
		})
		if err != nil {
			return FailWith(500, err.Error(), c)
		}

		return SucceedWith(map[string]interface{}{}, c)
	}
}

// GetMemberHandler is the handler responsible for retrieving a member score and rank
func GetMemberHandler(app *App) func(c echo.Context) error {
	return func(c echo.Context) error {
		leaderboardID := c.Param("leaderboardID")
		memberPublicID := c.Param("memberPublicID")

		lg := app.Logger.With(
			zap.String("handler", "GetMemberHandler"),
			zap.String("leaderboard", leaderboardID),
			zap.String("memberPublicID", memberPublicID),
		)

		order := c.QueryParam("order")
		if order == "" || (order != "asc" && order != "desc") {
			order = "desc"
		}

		scoreTTL := c.QueryParam("scoreTTL") == "true"
		var member *leaderboard.Member
		status := 404
		err := WithSegment("Model", c, func() error {
			var err error
			lg.Debug("Getting member.")
			l := leaderboard.NewLeaderboard(app.RedisClient.Trace(c.StdContext()), leaderboardID, 0)
			member, err = l.GetMember(memberPublicID, order, scoreTTL)
			if err != nil && strings.HasPrefix(err.Error(), notFoundError) {
				lg.Error("Member not found.", zap.Error(err))
				app.AddError()
				status = 404
				return fmt.Errorf("Member not found.")
			} else if err != nil {
				lg.Error("Get member failed.")
				app.AddError()
				status = 500
				return err
			}
			lg.Debug("Getting member succeeded.")
			return nil
		})
		if err != nil {
			return FailWith(status, err.Error(), c)
		}

		return SucceedWith(serializeMember(member, -1, scoreTTL), c)
	}
}

// GetMemberRankHandler is the handler responsible for retrieving a member rank
func GetMemberRankHandler(app *App) func(c echo.Context) error {
	return func(c echo.Context) error {
		leaderboardID := c.Param("leaderboardID")
		memberPublicID := c.Param("memberPublicID")
		lg := app.Logger.With(
			zap.String("handler", "GetMemberRankHandler"),
			zap.String("leaderboard", leaderboardID),
			zap.String("memberPublicID", memberPublicID),
		)

		order := c.QueryParam("order")
		if order == "" || (order != "asc" && order != "desc") {
			order = "desc"
		}

		status := 404
		rank := 0
		err := WithSegment("Model", c, func() error {
			var err error
			lg.Debug("Getting rank.")
			l := leaderboard.NewLeaderboard(app.RedisClient.Trace(c.StdContext()), leaderboardID, 0)
			rank, err = l.GetRank(memberPublicID, order)

			if err != nil && strings.HasPrefix(err.Error(), notFoundError) {
				lg.Error("Member not found.", zap.Error(err))
				app.AddError()
				status = 404
				return fmt.Errorf("Member not found.")
			} else if err != nil {
				lg.Error("Getting rank failed.", zap.Error(err))
				app.AddError()
				status = 500
				return err
			}
			lg.Debug("Getting rank succeeded.")
			return nil
		})
		if err != nil {
			return FailWith(status, err.Error(), c)
		}

		return SucceedWith(map[string]interface{}{
			"publicID": memberPublicID,
			"rank":     rank,
		}, c)
	}
}

//GetMemberRankInManyLeaderboardsHandler returns the member rank in several leaderboards at once
func GetMemberRankInManyLeaderboardsHandler(app *App) func(c echo.Context) error {
	return func(c echo.Context) error {
		memberPublicID := c.Param("memberPublicID")
		lg := app.Logger.With(
			zap.String("handler", "GetMemberRankInManyLeaderboardsHandler"),
			zap.String("memberPublicID", memberPublicID),
		)

		ids := c.QueryParam("leaderboardIds")
		order := c.QueryParam("order")
		if order == "" || (order != "asc" && order != "desc") {
			order = "desc"
		}
		scoreTTL := c.QueryParam("scoreTTL") == "true"

		if ids == "" {
			app.AddError()
			return FailWith(400, "Leaderboard IDs are required using the 'leaderboardIds' querystring parameter", c)
		}

		leaderboardIDs := strings.Split(ids, ",")
		serializedScores := make([]map[string]interface{}, len(leaderboardIDs))

		status := 404
		err := WithSegment("Model", c, func() error {
			for i, leaderboardID := range leaderboardIDs {
				lg.Debug("Getting member rank on leaderboard.", zap.String("leaderboard", leaderboardID))
				l := leaderboard.NewLeaderboard(app.RedisClient.Trace(c.StdContext()), leaderboardID, 0)
				member, err := l.GetMember(memberPublicID, order, scoreTTL)
				if err != nil && strings.HasPrefix(err.Error(), notFoundError) {
					lg.Error("Member not found.", zap.Error(err))
					app.AddError()
					status = 404
					return fmt.Errorf("Leaderboard not found or member not found in leaderboard.")
				} else if err != nil {
					lg.Error("Getting member rank on leaderboard failed.", zap.Error(err))
					app.AddError()
					status = 500
					return err
				}
				lg.Debug("Getting member rank on leaderboard succeeded.")
				serializedScores[i] = map[string]interface{}{
					"leaderboardID": leaderboardID,
					"rank":          member.Rank,
					"score":         member.Score,
				}
				if scoreTTL {
					serializedScores[i]["expireAt"] = member.ExpireAt
				}
			}
			return nil
		})
		if err != nil {
			return FailWith(status, err.Error(), c)
		}

		return SucceedWith(map[string]interface{}{
			"scores": serializedScores,
		}, c)
	}
}

// GetAroundMemberHandler retrieves a list of member score and rank centered in the given member
func GetAroundMemberHandler(app *App) func(c echo.Context) error {
	return func(c echo.Context) error {
		leaderboardID := c.Param("leaderboardID")
		memberPublicID := c.Param("memberPublicID")
		lg := app.Logger.With(
			zap.String("handler", "GetAroundMemberHandler"),
			zap.String("leaderboard", leaderboardID),
			zap.String("memberPublicID", memberPublicID),
		)

		order := c.QueryParam("order")
		if order == "" || (order != "asc" && order != "desc") {
			order = "desc"
		}
		getLastIfNotFound := false
		getLastIfNotFoundStr := c.QueryParam("getLastIfNotFound")
		if getLastIfNotFoundStr == "true" {
			getLastIfNotFound = true
		}

		pageSize, err := GetPageSize(app, c, defaultPageSize)
		if err != nil {
			return FailWith(400, err.Error(), c)
		}

		var members leaderboard.Members
		status := 404
		err = WithSegment("Model", c, func() error {
			lg.Debug("Getting members around player.")
			l := leaderboard.NewLeaderboard(app.RedisClient.Trace(c.StdContext()), leaderboardID, pageSize)
			members, err = l.GetAroundMe(memberPublicID, order, getLastIfNotFound)
			if err != nil && strings.HasPrefix(err.Error(), notFoundError) {
				lg.Error("Member not found.", zap.Error(err))
				app.AddError()
				status = 404
				return fmt.Errorf("Member not found.")
			} else if err != nil {
				lg.Error("Getting members around player failed.", zap.Error(err))
				app.AddError()
				status = 500
				return err
			}
			lg.Debug("Getting members around player succeeded.")
			return nil
		})
		if err != nil {
			return FailWith(status, err.Error(), c)
		}

		return SucceedWith(map[string]interface{}{
			"members": serializeMembers(members, false, false),
		}, c)
	}
}

// GetAroundScoreHandler retrieves a list of member scores and ranks centered in a given score
func GetAroundScoreHandler(app *App) func(c echo.Context) error {
	return func(c echo.Context) error {
		leaderboardID := c.Param("leaderboardID")
		lg := app.Logger.With(
			zap.String("handler", "GetAroundScoreHandler"),
			zap.String("leaderboard", leaderboardID),
		)

		order := c.QueryParam("order")
		if order == "" || (order != "asc" && order != "desc") {
			order = "desc"
		}

		score, err := strconv.ParseInt(c.Param("score"), 10, 64)
		if err != nil {
			return FailWith(400, "Score not sent or wrongly formatted", c)
		}

		pageSize, err := GetPageSize(app, c, defaultPageSize)
		if err != nil {
			return FailWith(400, err.Error(), c)
		}

		var members leaderboard.Members
		status := 404
		err = WithSegment("Model", c, func() error {
			lg.Debug("Getting players around score.", zap.Int64("score", score))
			l := leaderboard.NewLeaderboard(app.RedisClient.Trace(c.StdContext()), leaderboardID, pageSize)
			members, err = l.GetAroundScore(score, order)
			if err != nil && strings.HasPrefix(err.Error(), notFoundError) {
				lg.Error("Member not found.", zap.Error(err))
				app.AddError()
				status = 404
				return fmt.Errorf("Member not found.")
			} else if err != nil {
				lg.Error("Getting players around score failed.", zap.Error(err))
				app.AddError()
				status = 500
				return err
			}
			lg.Debug("Getting players around score succeeded.")
			return nil
		})
		if err != nil {
			return FailWith(status, err.Error(), c)
		}

		return SucceedWith(map[string]interface{}{
			"members": serializeMembers(members, false, false),
		}, c)
	}
}

// GetTotalMembersHandler is the handler responsible for returning the total number of members in a leaderboard
func GetTotalMembersHandler(app *App) func(c echo.Context) error {
	return func(c echo.Context) error {
		leaderboardID := c.Param("leaderboardID")
		lg := app.Logger.With(
			zap.String("handler", "GetTotalMembersHandler"),
			zap.String("leaderboard", leaderboardID),
		)

		count := 0
		err := WithSegment("Model", c, func() error {
			var err error
			lg.Debug("Getting total members.")
			l := leaderboard.NewLeaderboard(app.RedisClient.Trace(c.StdContext()), leaderboardID, 0)
			count, err = l.TotalMembers()

			if err != nil {
				lg.Error("Getting total members failed.", zap.Error(err))
				app.AddError()
				return err
			}
			lg.Debug("Getting total members succeeded.")
			return nil
		})
		if err != nil {
			return FailWith(500, err.Error(), c)
		}

		return SucceedWith(map[string]interface{}{
			"count": count,
		}, c)
	}
}

// GetTopMembersHandler retrieves onePage of member score and rank
func GetTopMembersHandler(app *App) func(c echo.Context) error {
	return func(c echo.Context) error {
		leaderboardID := c.Param("leaderboardID")
		lg := app.Logger.With(
			zap.String("handler", "GetTopMembersHandler"),
			zap.String("leaderboard", leaderboardID),
		)

		order := c.QueryParam("order")
		if order == "" || (order != "asc" && order != "desc") {
			order = "desc"
		}

		pageNumber, err := GetIntRouteParam(app, c, "pageNumber", 1)
		if err != nil {
			app.AddError()
			return FailWith(400, err.Error(), c)
		}

		pageSize, err := GetPageSize(app, c, defaultPageSize)
		if err != nil {
			return FailWith(400, err.Error(), c)
		}

		var members leaderboard.Members
		err = WithSegment("Model", c, func() error {
			lg.Debug("Getting top members.")
			l := leaderboard.NewLeaderboard(app.RedisClient.Trace(c.StdContext()), leaderboardID, pageSize)
			members, err = l.GetLeaders(pageNumber, order)

			if err != nil {
				lg.Error("Getting top members failed.", zap.Error(err))
				app.AddError()
				return err
			}
			lg.Debug("Getting top members succeeded.")
			return nil
		})
		if err != nil {
			return FailWith(500, err.Error(), c)
		}

		return SucceedWith(map[string]interface{}{
			"members": serializeMembers(members, false, false),
		}, c)
	}
}

// GetTopPercentageHandler retrieves top x % members in the leaderboard
func GetTopPercentageHandler(app *App) func(c echo.Context) error {
	return func(c echo.Context) error {
		leaderboardID := c.Param("leaderboardID")
		lg := app.Logger.With(
			zap.String("handler", "GetTopPercentageHandler"),
			zap.String("leaderboard", leaderboardID),
		)

		order := c.QueryParam("order")
		if order == "" || (order != "asc" && order != "desc") {
			order = "desc"
		}

		percentageStr := c.Param("percentage")
		percentage, err := strconv.ParseInt(percentageStr, 10, 32)
		if err != nil {
			app.AddError()
			return FailWith(400, fmt.Sprintf("Invalid percentage provided: %s", err.Error()), c)
		}
		if percentage == 0 {
			app.AddError()
			return FailWith(400, "Percentage must be a valid integer between 1 and 100.", c)
		}

		var members leaderboard.Members
		status := 400
		err = WithSegment("Model", c, func() error {
			lg.Debug("Getting top percentage.", zap.Int64("percentage", percentage))
			l := leaderboard.NewLeaderboard(app.RedisClient.Trace(c.StdContext()), leaderboardID, defaultPageSize)
			members, err = l.GetTopPercentage(int(percentage), app.Config.GetInt("api.maxReturnedMembers"), order)

			if err != nil {
				lg.Error("Getting top percentage failed.", zap.Error(err))
				if err.Error() == "Percentage must be a valid integer between 1 and 100." {
					app.AddError()
					status = 400
					return err
				}

				app.AddError()
				status = 500
				return err
			}
			lg.Debug("Getting top percentage succeeded.")
			return nil
		})
		if err != nil {
			return FailWith(status, err.Error(), c)
		}

		return SucceedWith(map[string]interface{}{
			"members": serializeMembers(members, false, false),
		}, c)
	}
}

// GetMembersHandler retrieves several members at once
func GetMembersHandler(app *App) func(c echo.Context) error {
	return func(c echo.Context) error {
		leaderboardID := c.Param("leaderboardID")
		lg := app.Logger.With(
			zap.String("handler", "GetMembersHandler"),
			zap.String("leaderboard", leaderboardID),
		)

		order := c.QueryParam("order")
		if order == "" || (order != "asc" && order != "desc") {
			order = "desc"
		}
		scoreTTL := c.QueryParam("scoreTTL") == "true"

		ids := c.QueryParam("ids")
		if ids == "" {
			app.AddError()
			return FailWith(400, "Member IDs are required using the 'ids' querystring parameter", c)
		}

		memberIDs := strings.Split(ids, ",")

		var members leaderboard.Members
		err := WithSegment("Model", c, func() error {
			var err error
			lg.Debug("Getting members.", zap.String("ids", ids))
			l := leaderboard.NewLeaderboard(app.RedisClient.Trace(c.StdContext()), leaderboardID, defaultPageSize)
			members, err = l.GetMembers(memberIDs, order, scoreTTL)

			if err != nil {
				lg.Error("Getting members failed.", zap.Error(err))
				app.AddError()
				return err
			}
			lg.Debug("Getting members succeeded.")
			return nil
		})
		if err != nil {
			return FailWith(500, err.Error(), c)
		}

		notFound := []string{}

		for _, memberID := range memberIDs {
			found := false
			for _, member := range members {
				if member.PublicID == memberID {
					found = true
					break
				}
			}
			if !found {
				notFound = append(notFound, memberID)
			}
		}

		return SucceedWith(map[string]interface{}{
			"members":  serializeMembers(members, true, scoreTTL),
			"notFound": notFound,
		}, c)
	}
}

// UpsertMemberLeaderboardsScoreHandler sets the member score for all leaderboards
func UpsertMemberLeaderboardsScoreHandler(app *App) func(c echo.Context) error {
	return func(c echo.Context) error {
		memberPublicID := c.Param("memberPublicID")
		lg := app.Logger.With(
			zap.String("handler", "UpsertMemberLeaderboardsScoreHandler"),
			zap.String("memberPublicID", memberPublicID),
		)

		scoreTTL := c.QueryParam("scoreTTL")

		var payload setScoresPayload

		prevRank := false
		prevRankStr := c.QueryParam("prevRank")
		if prevRankStr != "" && prevRankStr == "true" {
			prevRank = true
		}

		err := WithSegment("Payload", c, func() error {
			b, err := GetRequestBody(c)
			if err != nil {
				app.AddError()
				return err
			}
			if _, err := jsonparser.GetInt(b, "score"); err != nil {
				app.AddError()
				return fmt.Errorf("score is required")
			}
			if err := LoadJSONPayload(&payload, c, lg); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return FailWith(400, err.Error(), c)
		}

		serializedScores := make([]map[string]interface{}, len(payload.Leaderboards))

		err = WithSegment("Model", c, func() error {
			for i, leaderboardID := range payload.Leaderboards {
				lg.Debug("Updating score.",
					zap.String("leaderboardID", leaderboardID),
					zap.Int64("score", payload.Score))
				l := leaderboard.NewLeaderboard(app.RedisClient.Trace(c.StdContext()), leaderboardID, 0)
				member, err := l.SetMemberScore(memberPublicID, payload.Score, prevRank, scoreTTL)

				if err != nil {
					lg.Error("Update score failed.", zap.Error(err))
					app.AddError()
					return err
				}
				serializedScore := serializeMember(member, -1, scoreTTL != "")
				serializedScore["leaderboardID"] = leaderboardID
				serializedScores[i] = serializedScore
			}
			lg.Debug("Update score succeeded.")
			return nil
		})
		if err != nil {
			return FailWith(500, err.Error(), c)
		}

		return SucceedWith(map[string]interface{}{
			"scores": serializedScores,
		}, c)
	}
}

// RemoveLeaderboardHandler is the handler responsible for removing a leaderboard
func RemoveLeaderboardHandler(app *App) func(c echo.Context) error {
	return func(c echo.Context) error {
		leaderboardID := c.Param("leaderboardID")
		lg := app.Logger.With(
			zap.String("handler", "RemoveLeaderboardHandler"),
			zap.String("leaderboard", leaderboardID),
		)

		err := WithSegment("Model", c, func() error {
			lg.Debug("Removing leaderboard.")
			l := leaderboard.NewLeaderboard(app.RedisClient.Trace(c.StdContext()), leaderboardID, 0)
			err := l.RemoveLeaderboard()

			if err != nil {
				lg.Error("Remove leaderboard failed.", zap.Error(err))
				app.AddError()
				return err
			}
			lg.Debug("Remove leaderboard succeeeded.")
			return nil
		})
		if err != nil {
			return FailWith(500, err.Error(), c)
		}

		return SucceedWith(map[string]interface{}{}, c)
	}
}
