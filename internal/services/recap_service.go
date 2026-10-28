package services

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"streamgo/internal/models"
	"streamgo/internal/repository"
)

const (
	minPlayMs      = 30_000        // 30 seconds
	sessionGapSec  = 30 * 60       // 30 minutes
	ongoingTTLSec  = 15 * 60       // 15 minutes
	finishedTTLSec = 24 * 3600     // 24 hours
)

var (
	nightHours = map[int]bool{21: true, 22: true, 23: true, 0: true, 1: true, 2: true, 3: true}
	earlyHours = map[int]bool{5: true, 6: true, 7: true, 8: true}
)

// RecapService manages recap computation, caching, and sharing.
type RecapService struct {
	recapRepo repository.RecapRepository
	trackRepo repository.TrackRepository
}

// NewRecapService creates a new RecapService.
func NewRecapService(recapRepo repository.RecapRepository, trackRepo repository.TrackRepository) *RecapService {
	return &RecapService{
		recapRepo: recapRepo,
		trackRepo: trackRepo,
	}
}

func localMidnightUTC(year, month, day int, tzOffsetMin int) float64 {
	loc := time.FixedZone("user_tz", tzOffsetMin*60)
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, loc)
	return float64(t.Unix())
}

func userLocalTime(epoch float64, tzOffsetMin int) time.Time {
	loc := time.FixedZone("user_tz", tzOffsetMin*60)
	return time.Unix(int64(epoch), 0).In(loc)
}

// PeriodBounds calculates the start epoch, end epoch, and display label for a period ID.
func PeriodBounds(ptype models.RecapPeriodType, period string, tzOffsetMin int) (float64, float64, string, error) {
	switch ptype {
	case models.RecapPeriodWeekly:
		parts := strings.Split(strings.ToUpper(period), "-W")
		if len(parts) != 2 {
			return 0, 0, "", fmt.Errorf("invalid weekly period format '%s', expected YYYY-Www", period)
		}
		year, err1 := strconv.Atoi(parts[0])
		week, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil || week < 1 || week > 53 {
			return 0, 0, "", fmt.Errorf("invalid weekly period format '%s'", period)
		}

		// ISO week Monday: Jan 4 is always in week 1
		jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
		isoMondayWeek1 := jan4.AddDate(0, 0, -int((jan4.Weekday()+6)%7))
		monday := isoMondayWeek1.AddDate(0, 0, (week-1)*7)

		start := localMidnightUTC(monday.Year(), int(monday.Month()), monday.Day(), tzOffsetMin)
		nxtMonday := monday.AddDate(0, 0, 7)
		end := localMidnightUTC(nxtMonday.Year(), int(nxtMonday.Month()), nxtMonday.Day(), tzOffsetMin)

		sunday := monday.AddDate(0, 0, 6)
		label := fmt.Sprintf("%s %d – %s %d, %d", monday.Format("Jan"), monday.Day(), sunday.Format("Jan"), sunday.Day(), sunday.Year())
		return start, end, label, nil

	case models.RecapPeriodMonthly:
		parts := strings.Split(period, "-")
		if len(parts) != 2 {
			return 0, 0, "", fmt.Errorf("invalid monthly period format '%s', expected YYYY-MM", period)
		}
		year, err1 := strconv.Atoi(parts[0])
		month, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil || month < 1 || month > 12 {
			return 0, 0, "", fmt.Errorf("invalid monthly period format '%s'", period)
		}

		start := localMidnightUTC(year, month, 1, tzOffsetMin)
		ny, nm := year, month+1
		if month == 12 {
			ny, nm = year+1, 1
		}
		end := localMidnightUTC(ny, nm, 1, tzOffsetMin)
		label := fmt.Sprintf("%s %d", time.Month(month).String(), year)
		return start, end, label, nil

	case models.RecapPeriodYearly:
		year, err := strconv.Atoi(period)
		if err != nil {
			return 0, 0, "", fmt.Errorf("invalid yearly period format '%s', expected YYYY", period)
		}
		start := localMidnightUTC(year, 1, 1, tzOffsetMin)
		end := localMidnightUTC(year+1, 1, 1, tzOffsetMin)
		label := fmt.Sprintf("%d", year)
		return start, end, label, nil

	default:
		return 0, 0, "", fmt.Errorf("unknown period type '%s'", ptype)
	}
}

// PreviousPeriod returns the identifier of the period immediately preceding the given period.
func PreviousPeriod(ptype models.RecapPeriodType, period string) string {
	switch ptype {
	case models.RecapPeriodWeekly:
		parts := strings.Split(strings.ToUpper(period), "-W")
		if len(parts) == 2 {
			year, _ := strconv.Atoi(parts[0])
			week, _ := strconv.Atoi(parts[1])
			jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
			isoMondayWeek1 := jan4.AddDate(0, 0, -int((jan4.Weekday()+6)%7))
			monday := isoMondayWeek1.AddDate(0, 0, (week-1)*7).AddDate(0, 0, -7)
			prevYear, prevWeek := monday.ISOWeek()
			return fmt.Sprintf("%d-W%02d", prevYear, prevWeek)
		}
		return period

	case models.RecapPeriodMonthly:
		parts := strings.Split(period, "-")
		if len(parts) == 2 {
			year, _ := strconv.Atoi(parts[0])
			month, _ := strconv.Atoi(parts[1])
			if month == 1 {
				return fmt.Sprintf("%d-12", year-1)
			}
			return fmt.Sprintf("%d-%02d", year, month-1)
		}
		return period

	case models.RecapPeriodYearly:
		year, err := strconv.Atoi(period)
		if err == nil {
			return strconv.Itoa(year - 1)
		}
		return period

	default:
		return period
	}
}

// PeriodIDFor formats a timestamp into the period identifier for the given type.
func PeriodIDFor(ptype models.RecapPeriodType, t time.Time) string {
	switch ptype {
	case models.RecapPeriodWeekly:
		year, week := t.ISOWeek()
		return fmt.Sprintf("%d-W%02d", year, week)
	case models.RecapPeriodMonthly:
		return t.Format("2006-01")
	case models.RecapPeriodYearly:
		return fmt.Sprintf("%d", t.Year())
	default:
		return ""
	}
}

// IsPeriodAvailable determines whether a recap period is ready/unlocked to show to users.
func IsPeriodAvailable(ptype models.RecapPeriodType, period string, tzOffsetMin int) bool {
	_, end, _, err := PeriodBounds(ptype, period, tzOffsetMin)
	if err != nil {
		return false
	}

	now := float64(time.Now().Unix())
	if end <= now {
		return true // past completed period is always available
	}

	nowLocal := userLocalTime(now, tzOffsetMin)
	switch ptype {
	case models.RecapPeriodWeekly:
		// Unlocked on weekends (Saturday or Sunday)
		wd := nowLocal.Weekday()
		return wd == time.Saturday || wd == time.Sunday
	case models.RecapPeriodMonthly:
		// Unlocked on the last day of the month
		nextDay := nowLocal.AddDate(0, 0, 1)
		return nextDay.Month() != nowLocal.Month()
	case models.RecapPeriodYearly:
		// Unlocked on Dec 31st
		return nowLocal.Month() == time.December && nowLocal.Day() == 31
	default:
		return false
	}
}

func classifyEvent(ev *models.ListeningEventDoc, durationMs int) (isPlay bool, completed bool, isSkip bool, playedMs int) {
	if ev.PlayedMs == nil {
		// Legacy history row: assume played through
		if durationMs > 0 {
			playedMs = durationMs
		} else {
			playedMs = 180_000 // 3 minutes default
		}
	} else {
		playedMs = *ev.PlayedMs
	}

	completed = ev.Completed || (durationMs > 0 && playedMs >= int(0.8*float64(durationMs)))
	isPlay = playedMs >= minPlayMs || completed
	isSkip = !isPlay

	effectiveMs := playedMs
	if durationMs > 0 && playedMs > durationMs {
		effectiveMs = durationMs
	}
	return isPlay, completed, isSkip, effectiveMs
}

type startSession struct {
	start float64
	ms    int
}

// ComputeStats calculates aggregated listening statistics for a list of events.
func (s *RecapService) ComputeStats(
	events []*models.ListeningEventDoc,
	tracks map[string]*models.Track,
	tzOffsetMin int,
	seenBefore map[string]bool,
) *models.RecapStats {
	totalMs := 0
	plays := 0
	completed := 0
	skipped := 0

	trackPlays := make(map[string]int)
	trackMs := make(map[string]int)

	artistPlays := make(map[string]int)
	artistMs := make(map[string]int)
	artistCovers := make(map[string]string)

	type albumKey struct {
		album  string
		artist string
	}
	albumPlays := make(map[albumKey]int)
	albumMs := make(map[albumKey]int)
	albumMeta := make(map[albumKey]*models.AlbumStat)

	topicPlays := make(map[string]int)

	byHour := make([]int, 24)
	byWeekday := make([]int, 7)
	byDate := make(map[string]int)
	byMonth := make(map[string]int)

	losslessPlays := 0
	var starts []startSession

	for _, ev := range events {
		t := tracks[ev.TrackID]
		durationMs := ev.DurationMs
		if durationMs <= 0 && t != nil && t.Audio.DurationSec > 0 {
			durationMs = int(t.Audio.DurationSec) * 1000
		}

		_, isComp, isSkip, playedMs := classifyEvent(ev, durationMs)
		if isSkip {
			skipped++
			continue
		}

		plays++
		if isComp {
			completed++
		}
		totalMs += playedMs

		tid := ev.TrackID
		trackPlays[tid]++
		trackMs[tid] += playedMs

		when := userLocalTime(ev.PlayedAt, tzOffsetMin)
		hour := when.Hour()
		if hour >= 0 && hour < 24 {
			byHour[hour] += playedMs
		}

		// Convert Go Sunday=0 weekday to ISO Monday=0 ... Sunday=6
		isoWd := int((when.Weekday() + 6) % 7)
		if isoWd >= 0 && isoWd < 7 {
			byWeekday[isoWd] += playedMs
		}

		dateKey := when.Format("2006-01-02")
		byDate[dateKey] += playedMs

		monthKey := when.Format("2006-01")
		byMonth[monthKey] += playedMs

		startTime := ev.StartedAt
		if startTime <= 0 {
			startTime = ev.PlayedAt
		}
		starts = append(starts, startSession{start: startTime, ms: playedMs})

		if t != nil {
			artist := strings.TrimSpace(t.EffectiveArtist())
			if artist == "" {
				artist = "Unknown artist"
			}
			artistPlays[artist]++
			artistMs[artist] += playedMs
			if _, exists := artistCovers[artist]; !exists {
				artistCovers[artist] = t.EffectiveCoverURL()
			}

			if album := strings.TrimSpace(t.Audio.Album); album != "" {
				ak := albumKey{album: album, artist: artist}
				albumPlays[ak]++
				albumMs[ak] += playedMs
				if _, exists := albumMeta[ak]; !exists {
					var albumIDPtr *string
					if t.Audio.AlbumID != "" {
						idCopy := t.Audio.AlbumID
						albumIDPtr = &idCopy
					}
					var coverPtr *string
					if cov := t.EffectiveCoverURL(); cov != "" {
						covCopy := cov
						coverPtr = &covCopy
					}
					albumMeta[ak] = &models.AlbumStat{
						Album:    album,
						Artist:   artist,
						AlbumID:  albumIDPtr,
						CoverURL: coverPtr,
					}
				}
			}

			if t.TopicName != "" {
				topicPlays[t.TopicName]++
			}

			typ := strings.ToUpper(strings.TrimSpace(t.Audio.Type))
			if typ == "FLAC" || typ == "ALAC" || typ == "WAV" {
				losslessPlays++
			}
		}
	}

	// Longest session & session count calculation (split on gaps > 30 min)
	longestSessionMs := 0
	sessionCount := 0
	if len(starts) > 0 {
		sort.Slice(starts, func(i, j int) bool {
			return starts[i].start < starts[j].start
		})
		curStart := starts[0].start
		curMs := starts[0].ms
		curEnd := curStart + float64(starts[0].ms)/1000.0
		sessionCount = 1

		for _, s := range starts[1:] {
			if s.start-curEnd > sessionGapSec {
				if curMs > longestSessionMs {
					longestSessionMs = curMs
				}
				curStart = s.start
				curMs = 0
				sessionCount++
			}
			curMs += s.ms
			end := s.start + float64(s.ms)/1000.0
			if end > curEnd {
				curEnd = end
			}
		}
		if curMs > longestSessionMs {
			longestSessionMs = curMs
		}
	}

	// Build Top Tracks
	type trackEntry struct {
		tid   string
		plays int
		ms    int
	}
	var sortedTracks []trackEntry
	for tid, count := range trackPlays {
		sortedTracks = append(sortedTracks, trackEntry{tid: tid, plays: count, ms: trackMs[tid]})
	}
	sort.Slice(sortedTracks, func(i, j int) bool {
		if sortedTracks[i].plays == sortedTracks[j].plays {
			return sortedTracks[i].ms > sortedTracks[j].ms
		}
		return sortedTracks[i].plays > sortedTracks[j].plays
	})

	var topTracks []*models.TrackStat
	maxTopTracks := 10
	if len(sortedTracks) < maxTopTracks {
		maxTopTracks = len(sortedTracks)
	}
	for i := 0; i < maxTopTracks; i++ {
		te := sortedTracks[i]
		t := tracks[te.tid]
		var item *models.BrowseItem
		if t != nil {
			item = t.ToBrowseItem()
		} else {
			item = &models.BrowseItem{
				ID:    te.tid,
				Title: "Unknown track",
			}
		}
		topTracks = append(topTracks, &models.TrackStat{
			Track:   item,
			Plays:   te.plays,
			Minutes: math.Round(float64(te.ms)/60000.0*10) / 10,
		})
	}

	// Build Top Artists
	type artistEntry struct {
		name  string
		plays int
		ms    int
	}
	var sortedArtists []artistEntry
	for name, count := range artistPlays {
		sortedArtists = append(sortedArtists, artistEntry{name: name, plays: count, ms: artistMs[name]})
	}
	sort.Slice(sortedArtists, func(i, j int) bool {
		if sortedArtists[i].plays == sortedArtists[j].plays {
			return sortedArtists[i].ms > sortedArtists[j].ms
		}
		return sortedArtists[i].plays > sortedArtists[j].plays
	})

	var topArtists []*models.ArtistStat
	maxTopArtists := 10
	if len(sortedArtists) < maxTopArtists {
		maxTopArtists = len(sortedArtists)
	}
	for i := 0; i < maxTopArtists; i++ {
		ae := sortedArtists[i]
		var covPtr *string
		if c, ok := artistCovers[ae.name]; ok && c != "" {
			cCopy := c
			covPtr = &cCopy
		}
		topArtists = append(topArtists, &models.ArtistStat{
			Name:     ae.name,
			Plays:    ae.plays,
			Minutes:  math.Round(float64(ae.ms)/60000.0*10) / 10,
			CoverURL: covPtr,
		})
	}

	// Build Top Albums
	type albumEntry struct {
		key   albumKey
		plays int
		ms    int
	}
	var sortedAlbums []albumEntry
	for k, count := range albumPlays {
		sortedAlbums = append(sortedAlbums, albumEntry{key: k, plays: count, ms: albumMs[k]})
	}
	sort.Slice(sortedAlbums, func(i, j int) bool {
		if sortedAlbums[i].plays == sortedAlbums[j].plays {
			return sortedAlbums[i].ms > sortedAlbums[j].ms
		}
		return sortedAlbums[i].plays > sortedAlbums[j].plays
	})

	var topAlbums []*models.AlbumStat
	maxTopAlbums := 6
	if len(sortedAlbums) < maxTopAlbums {
		maxTopAlbums = len(sortedAlbums)
	}
	for i := 0; i < maxTopAlbums; i++ {
		ale := sortedAlbums[i]
		baseMeta := albumMeta[ale.key]
		topAlbums = append(topAlbums, &models.AlbumStat{
			Album:    baseMeta.Album,
			Artist:   baseMeta.Artist,
			AlbumID:  baseMeta.AlbumID,
			CoverURL: baseMeta.CoverURL,
			Plays:    ale.plays,
			Minutes:  math.Round(float64(ale.ms)/60000.0*10) / 10,
		})
	}

	// Top Genres
	type genreEntry struct {
		name  string
		plays int
	}
	var sortedGenres []genreEntry
	for name, count := range topicPlays {
		sortedGenres = append(sortedGenres, genreEntry{name: name, plays: count})
	}
	sort.Slice(sortedGenres, func(i, j int) bool {
		return sortedGenres[i].plays > sortedGenres[j].plays
	})

	var topGenres []*models.GenreStat
	maxTopGenres := 5
	if len(sortedGenres) < maxTopGenres {
		maxTopGenres = len(sortedGenres)
	}
	for i := 0; i < maxTopGenres; i++ {
		topGenres = append(topGenres, &models.GenreStat{
			Name:  sortedGenres[i].name,
			Plays: sortedGenres[i].plays,
		})
	}

	// Convert hour and weekday ms to minutes
	listeningByHour := make([]int, 24)
	for h := 0; h < 24; h++ {
		listeningByHour[h] = int(math.Round(float64(byHour[h]) / 60000.0))
	}
	listeningByWeekday := make([]int, 7)
	for d := 0; d < 7; d++ {
		listeningByWeekday[d] = int(math.Round(float64(byWeekday[d]) / 60000.0))
	}

	// Sorted days
	var sortedDayKeys []string
	for k := range byDate {
		sortedDayKeys = append(sortedDayKeys, k)
	}
	sort.Strings(sortedDayKeys)
	var listeningByDay []*models.DayStat
	for _, k := range sortedDayKeys {
		listeningByDay = append(listeningByDay, &models.DayStat{
			Date:    k,
			Minutes: int(math.Round(float64(byDate[k]) / 60000.0)),
		})
	}

	// Sorted months
	var sortedMonthKeys []string
	for k := range byMonth {
		sortedMonthKeys = append(sortedMonthKeys, k)
	}
	sort.Strings(sortedMonthKeys)
	var listeningByMonth []*models.MonthStat
	for _, k := range sortedMonthKeys {
		listeningByMonth = append(listeningByMonth, &models.MonthStat{
			Month:   k,
			Minutes: int(math.Round(float64(byMonth[k]) / 60000.0)),
		})
	}

	// Active periods & shares
	nightMs := 0
	for h := range nightHours {
		nightMs += byHour[h]
	}
	earlyMs := 0
	for h := range earlyHours {
		earlyMs += byHour[h]
	}
	weekendMs := byWeekday[5] + byWeekday[6]

	repeatPlays := 0
	for _, c := range trackPlays {
		if c > 1 {
			repeatPlays += c - 1
		}
	}

	var mostActiveHour *int
	if totalMs > 0 {
		bestH := 0
		bestHMs := byHour[0]
		for h := 1; h < 24; h++ {
			if byHour[h] > bestHMs {
				bestH = h
				bestHMs = byHour[h]
			}
		}
		mostActiveHour = &bestH
	}

	var mostActiveWeekday *int
	if totalMs > 0 {
		bestD := 0
		bestDMs := byWeekday[0]
		for d := 1; d < 7; d++ {
			if byWeekday[d] > bestDMs {
				bestD = d
				bestDMs = byWeekday[d]
			}
		}
		mostActiveWeekday = &bestD
	}

	var mostActiveDate *string
	mostActiveDateMinutes := 0
	if len(byDate) > 0 {
		bestDate := ""
		bestDateMs := -1
		for d, ms := range byDate {
			if ms > bestDateMs {
				bestDate = d
				bestDateMs = ms
			}
		}
		if bestDate != "" {
			mostActiveDate = &bestDate
			mostActiveDateMinutes = int(math.Round(float64(bestDateMs) / 60000.0))
		}
	}

	// Discovery tracks
	var discoveryTracks []*models.BrowseItem
	discoveryCount := 0
	for _, te := range sortedTracks {
		if !seenBefore[te.tid] {
			discoveryCount++
			if len(discoveryTracks) < 6 {
				if t := tracks[te.tid]; t != nil {
					discoveryTracks = append(discoveryTracks, t.ToBrowseItem())
				}
			}
		}
	}

	round3 := func(val float64) float64 {
		return math.Round(val*1000) / 1000
	}

	nightShare := 0.0
	earlyShare := 0.0
	weekendShare := 0.0
	if totalMs > 0 {
		nightShare = round3(float64(nightMs) / float64(totalMs))
		earlyShare = round3(float64(earlyMs) / float64(totalMs))
		weekendShare = round3(float64(weekendMs) / float64(totalMs))
	}

	losslessShare := 0.0
	if plays > 0 {
		losslessShare = round3(float64(losslessPlays) / float64(plays))
	}

	topArtistShare := 0.0
	if plays > 0 && len(sortedArtists) > 0 {
		topArtistShare = round3(float64(sortedArtists[0].plays) / float64(plays))
	}

	return &models.RecapStats{
		TotalMinutes:          int(math.Round(float64(totalMs) / 60000.0)),
		TotalPlays:            plays,
		CompletedPlays:        completed,
		SkippedPlays:          skipped,
		UniqueTracks:          len(trackPlays),
		UniqueArtists:         len(artistPlays),
		UniqueAlbums:          len(albumPlays),
		TopTracks:             topTracks,
		TopArtists:            topArtists,
		TopAlbums:             topAlbums,
		TopGenres:             topGenres,
		ListeningByHour:       listeningByHour,
		ListeningByWeekday:    listeningByWeekday,
		ListeningByDay:        listeningByDay,
		ListeningByMonth:      listeningByMonth,
		DiscoveryCount:        discoveryCount,
		DiscoveryTracks:       discoveryTracks,
		RepeatCount:           repeatPlays,
		MostActiveHour:        mostActiveHour,
		MostActiveWeekday:     mostActiveWeekday,
		MostActiveDate:        mostActiveDate,
		MostActiveDateMinutes: mostActiveDateMinutes,
		LongestSessionMinutes: int(math.Round(float64(longestSessionMs) / 60000.0)),
		SessionCount:          sessionCount,
		NightShare:            nightShare,
		EarlyShare:            earlyShare,
		WeekendShare:          weekendShare,
		LosslessShare:         losslessShare,
		TopArtistShare:        topArtistShare,
	}
}

// ComputePersonality derives up to 3 transparent listening personality traits.
func ComputePersonality(stats *models.RecapStats) []*models.RecapPersonality {
	var out []*models.RecapPersonality
	plays := stats.TotalPlays
	if plays < 5 {
		return out
	}

	if stats.NightShare >= 0.45 {
		out = append(out, &models.RecapPersonality{
			ID:     "night_owl",
			Name:   "Night Owl",
			Detail: fmt.Sprintf("%d%% of your listening happened after 9 PM", int(math.Round(stats.NightShare*100))),
		})
	}
	if stats.EarlyShare >= 0.35 {
		out = append(out, &models.RecapPersonality{
			ID:     "early_bird",
			Name:   "Early Bird",
			Detail: fmt.Sprintf("%d%% of your listening was before 9 AM", int(math.Round(stats.EarlyShare*100))),
		})
	}
	if stats.UniqueTracks > 0 && float64(stats.DiscoveryCount)/float64(stats.UniqueTracks) >= 0.5 && stats.DiscoveryCount >= 10 {
		out = append(out, &models.RecapPersonality{
			ID:     "explorer",
			Name:   "Explorer",
			Detail: fmt.Sprintf("%d of %d tracks were new to you", stats.DiscoveryCount, stats.UniqueTracks),
		})
	}
	if plays > 0 && float64(stats.RepeatCount)/float64(plays) >= 0.45 {
		out = append(out, &models.RecapPersonality{
			ID:     "replayer",
			Name:   "Replayer",
			Detail: fmt.Sprintf("%d replays — you know what you like", stats.RepeatCount),
		})
	}
	if stats.TopArtistShare >= 0.4 && len(stats.TopArtists) > 0 {
		out = append(out, &models.RecapPersonality{
			ID:     "loyalist",
			Name:   "Loyalist",
			Detail: fmt.Sprintf("%d%% of plays were %s", int(math.Round(stats.TopArtistShare*100)), stats.TopArtists[0].Name),
		})
	}
	if stats.UniqueArtists >= 25 && stats.TopArtistShare < 0.15 {
		out = append(out, &models.RecapPersonality{
			ID:     "genre_hopper",
			Name:   "Genre Hopper",
			Detail: fmt.Sprintf("%d artists, no clear favourite", stats.UniqueArtists),
		})
	}
	if stats.UniqueAlbums > 0 && len(stats.TopAlbums) > 0 && stats.TopAlbums[0].Plays >= 8 && plays > 0 && float64(stats.CompletedPlays)/float64(plays) >= 0.7 {
		out = append(out, &models.RecapPersonality{
			ID:     "album_collector",
			Name:   "Album Listener",
			Detail: "You play albums through, front to back",
		})
	}
	if stats.WeekendShare >= 0.5 {
		out = append(out, &models.RecapPersonality{
			ID:     "weekend",
			Name:   "Weekend Listener",
			Detail: fmt.Sprintf("%d%% of listening on weekends", int(math.Round(stats.WeekendShare*100))),
		})
	}
	if stats.LongestSessionMinutes >= 120 {
		out = append(out, &models.RecapPersonality{
			ID:     "binge",
			Name:   "Binge Listener",
			Detail: fmt.Sprintf("Longest session: %d minutes", stats.LongestSessionMinutes),
		})
	}
	if stats.LosslessShare >= 0.6 {
		out = append(out, &models.RecapPersonality{
			ID:     "audiophile",
			Name:   "Audiophile",
			Detail: fmt.Sprintf("%d%% of plays were lossless", int(math.Round(stats.LosslessShare*100))),
		})
	}

	if len(out) == 0 {
		out = append(out, &models.RecapPersonality{
			ID:     "steady",
			Name:   "Steady Listener",
			Detail: fmt.Sprintf("%d plays across %d artists", plays, stats.UniqueArtists),
		})
	}

	if len(out) > 3 {
		return out[:3]
	}
	return out
}

// BuildSnapshot generates a full RecapSnapshot for a user and period.
func (s *RecapService) BuildSnapshot(ctx context.Context, userID int64, ptype models.RecapPeriodType, period string, tzOffsetMin int) (*models.RecapSnapshot, error) {
	start, end, label, err := PeriodBounds(ptype, period, tzOffsetMin)
	if err != nil {
		return nil, err
	}

	events, err := s.recapRepo.LoadEvents(ctx, userID, start, end)
	if err != nil {
		return nil, err
	}

	// Extract unique track IDs
	var trackIDs []string
	idSet := make(map[string]bool)
	for _, e := range events {
		if e.TrackID != "" && !idSet[e.TrackID] {
			idSet[e.TrackID] = true
			trackIDs = append(trackIDs, e.TrackID)
		}
	}

	// Load track metadata from MongoDB
	tracksMap := make(map[string]*models.Track)
	if len(trackIDs) > 0 {
		loadedTracks, err := s.trackRepo.GetByIDs(ctx, trackIDs)
		if err == nil {
			for _, t := range loadedTracks {
				tracksMap[t.ID] = t
			}
		}
	}

	seenBefore, _ := s.recapRepo.FirstSeenBefore(ctx, userID, trackIDs, start)
	stats := s.ComputeStats(events, tracksMap, tzOffsetMin, seenBefore)

	// Previous period comparison
	prevID := PreviousPeriod(ptype, period)
	var comparison *models.RecapComparison
	if prevStart, prevEnd, _, err := PeriodBounds(ptype, prevID, tzOffsetMin); err == nil {
		prevEvents, err := s.recapRepo.LoadEvents(ctx, userID, prevStart, prevEnd)
		if err == nil && len(prevEvents) > 0 {
			var prevTrackIDs []string
			prevIDSet := make(map[string]bool)
			for _, e := range prevEvents {
				if e.TrackID != "" && !prevIDSet[e.TrackID] {
					prevIDSet[e.TrackID] = true
					prevTrackIDs = append(prevTrackIDs, e.TrackID)
				}
			}
			prevTracksMap := make(map[string]*models.Track)
			if loaded, err := s.trackRepo.GetByIDs(ctx, prevTrackIDs); err == nil {
				for _, t := range loaded {
					prevTracksMap[t.ID] = t
				}
			}

			prevStats := s.ComputeStats(prevEvents, prevTracksMap, tzOffsetMin, make(map[string]bool))
			comparison = ComputeComparison(prevID, stats, prevStats)
		}
	}

	now := float64(time.Now().Unix())
	legacyData := len(events) > 0
	for _, e := range events {
		if !e.Legacy {
			legacyData = false
			break
		}
	}

	return &models.RecapSnapshot{
		SchemaVersion: repository.RecapSchemaVersion,
		Type:          ptype,
		Period:        period,
		Label:         label,
		Start:         start,
		End:           end,
		Ongoing:       end > now,
		GeneratedAt:   now,
		TzOffsetMin:   tzOffsetMin,
		EventCount:    len(events),
		LegacyData:    legacyData,
		Stats:         stats,
		Personality:   ComputePersonality(stats),
		Comparison:    comparison,
	}, nil
}

// GetSnapshot returns a cached snapshot if valid, or builds and saves a fresh one.
func (s *RecapService) GetSnapshot(ctx context.Context, userID int64, ptype models.RecapPeriodType, period string, tzOffsetMin int, force bool) (*models.RecapSnapshot, error) {
	now := float64(time.Now().Unix())
	if !force {
		cached, genAt, err := s.recapRepo.GetSnapshot(ctx, userID, ptype, period, tzOffsetMin)
		if err == nil && cached != nil {
			ttl := float64(finishedTTLSec)
			if cached.Ongoing {
				ttl = float64(ongoingTTLSec)
			}
			if now-genAt < ttl {
				return cached, nil
			}
		}
	}

	snap, err := s.BuildSnapshot(ctx, userID, ptype, period, tzOffsetMin)
	if err != nil {
		return nil, err
	}

	_ = s.recapRepo.SaveSnapshot(ctx, userID, ptype, period, tzOffsetMin, snap)
	return snap, nil
}

// ListAvailable returns all unlocked periods that contain listening history for the user.
func (s *RecapService) ListAvailable(ctx context.Context, userID int64, tzOffsetMin int) ([]*models.RecapAvailableItem, error) {
	firstTimestamp, found, err := s.recapRepo.FirstEventTimestamp(ctx, userID)
	if err != nil || !found || firstTimestamp <= 0 {
		return []*models.RecapAvailableItem{}, nil
	}

	now := float64(time.Now().Unix())
	nowLocal := userLocalTime(now, tzOffsetMin)
	firstLocal := userLocalTime(firstTimestamp, tzOffsetMin)
	var out []*models.RecapAvailableItem

	// Check weeks (up to 8 weeks backwards)
	curWeek := nowLocal
	for i := 0; i < 8; i++ {
		pid := PeriodIDFor(models.RecapPeriodWeekly, curWeek)
		start, end, label, err := PeriodBounds(models.RecapPeriodWeekly, pid, tzOffsetMin)
		if err == nil {
			if end < firstTimestamp {
				break
			}
			if IsPeriodAvailable(models.RecapPeriodWeekly, pid, tzOffsetMin) {
				has, _ := s.recapRepo.HasPlays(ctx, userID, start, end)
				if has {
					out = append(out, &models.RecapAvailableItem{
						Type:    models.RecapPeriodWeekly,
						Period:  pid,
						Label:   label,
						Ongoing: end > now,
					})
				}
			}
		}
		curWeek = curWeek.AddDate(0, 0, -7)
	}

	// Check months (up to 12 months backwards)
	y, m := nowLocal.Year(), int(nowLocal.Month())
	for i := 0; i < 12; i++ {
		pid := fmt.Sprintf("%d-%02d", y, m)
		start, end, label, err := PeriodBounds(models.RecapPeriodMonthly, pid, tzOffsetMin)
		if err == nil {
			if end < firstTimestamp {
				break
			}
			if IsPeriodAvailable(models.RecapPeriodMonthly, pid, tzOffsetMin) {
				has, _ := s.recapRepo.HasPlays(ctx, userID, start, end)
				if has {
					out = append(out, &models.RecapAvailableItem{
						Type:    models.RecapPeriodMonthly,
						Period:  pid,
						Label:   label,
						Ongoing: end > now,
					})
				}
			}
		}
		if m == 1 {
			y, m = y-1, 12
		} else {
			m--
		}
	}

	// Check years
	for year := nowLocal.Year(); year >= firstLocal.Year(); year-- {
		pid := strconv.Itoa(year)
		start, end, label, err := PeriodBounds(models.RecapPeriodYearly, pid, tzOffsetMin)
		if err == nil {
			if IsPeriodAvailable(models.RecapPeriodYearly, pid, tzOffsetMin) {
				has, _ := s.recapRepo.HasPlays(ctx, userID, start, end)
				if has {
					out = append(out, &models.RecapAvailableItem{
						Type:    models.RecapPeriodYearly,
						Period:  pid,
						Label:   label,
						Ongoing: end > now,
					})
				}
			}
		}
	}

	return out, nil
}

// ComputeComparison calculates comparison statistics against the preceding period.
func ComputeComparison(prevID string, stats *models.RecapStats, prevStats *models.RecapStats) *models.RecapComparison {
	if stats == nil || prevStats == nil {
		return nil
	}

	deltaPct := func(cur, old int) *int {
		if old == 0 {
			return nil
		}
		pct := int(math.Round(float64(cur-old) / float64(old) * 100))
		return &pct
	}

	var topArtistPrev *string
	if len(prevStats.TopArtists) > 0 {
		aName := prevStats.TopArtists[0].Name
		topArtistPrev = &aName
	}

	return &models.RecapComparison{
		Period:          prevID,
		TotalMinutes:    prevStats.TotalMinutes,
		TotalPlays:      prevStats.TotalPlays,
		UniqueArtists:   prevStats.UniqueArtists,
		UniqueTracks:    prevStats.UniqueTracks,
		MinutesDeltaPct: deltaPct(stats.TotalMinutes, prevStats.TotalMinutes),
		PlaysDeltaPct:   deltaPct(stats.TotalPlays, prevStats.TotalPlays),
		ArtistsDelta:    stats.UniqueArtists - prevStats.UniqueArtists,
		NightShareDelta: int(math.Round((stats.NightShare - prevStats.NightShare) * 100)),
		TopArtistPrev:   topArtistPrev,
	}
}

// PublicSummary converts a full RecapSnapshot into a sanitized summary for public sharing.
func PublicSummary(snap *models.RecapSnapshot) *models.RecapPublicSummary {
	var topTrack *models.RecapPublicTrack
	if len(snap.Stats.TopTracks) > 0 && snap.Stats.TopTracks[0].Track != nil {
		t := snap.Stats.TopTracks[0].Track
		var tTitle, tArtist, tCover *string
		if t.Title != "" {
			c := t.Title
			tTitle = &c
		}
		if t.Artist != "" {
			c := t.Artist
			tArtist = &c
		}
		if t.CoverURL != "" {
			c := t.CoverURL
			tCover = &c
		}
		topTrack = &models.RecapPublicTrack{
			Title:    tTitle,
			Artist:   tArtist,
			CoverURL: tCover,
		}
	}

	var topArtist *models.ArtistStat
	if len(snap.Stats.TopArtists) > 0 {
		topArtist = snap.Stats.TopArtists[0]
	}

	return &models.RecapPublicSummary{
		Type:          snap.Type,
		Period:        snap.Period,
		Label:         snap.Label,
		TotalMinutes:  snap.Stats.TotalMinutes,
		TotalPlays:    snap.Stats.TotalPlays,
		UniqueArtists: snap.Stats.UniqueArtists,
		UniqueTracks:  snap.Stats.UniqueTracks,
		TopArtist:     topArtist,
		TopTrack:      topTrack,
		Personality:   snap.Personality,
	}
}

// CreateShare creates or refreshes a public share link for a user's recap.
func (s *RecapService) CreateShare(ctx context.Context, userID int64, snap *models.RecapSnapshot) (*models.RecapShareItem, error) {
	summary := PublicSummary(snap)
	return s.recapRepo.CreateShare(ctx, userID, snap.Type, snap.Period, summary)
}

// ListShares returns all active public share links for the user.
func (s *RecapService) ListShares(ctx context.Context, userID int64) ([]*models.RecapShareItem, error) {
	return s.recapRepo.ListShares(ctx, userID)
}

// RevokeShare revokes an active share token.
func (s *RecapService) RevokeShare(ctx context.Context, userID int64, token string) (bool, error) {
	return s.recapRepo.RevokeShare(ctx, userID, token)
}

// GetPublicShare returns the sanitized summary for a public share link.
func (s *RecapService) GetPublicShare(ctx context.Context, token string) (*models.RecapPublicSummary, error) {
	return s.recapRepo.GetPublicShare(ctx, token)
}

// DeleteUserData deletes all recap telemetry and share links for a user.
func (s *RecapService) DeleteUserData(ctx context.Context, userID int64) (map[string]int64, error) {
	e, sn, sh, err := s.recapRepo.DeleteUserData(ctx, userID)
	if err != nil {
		return nil, err
	}
	return map[string]int64{
		"events":    e,
		"snapshots": sn,
		"shares":    sh,
	}, nil
}

// RecordEvents saves incoming listening telemetry events.
func (s *RecapService) RecordEvents(ctx context.Context, userID int64, events []models.ListeningEventItem) (int, error) {
	return s.recapRepo.RecordEvents(ctx, userID, events)
}
