/*
 *   bowlbot - Organise bowling games with a Telegram bot
 *   Copyright (c) 2017 Shannon Wynter.
 *
 *   This program is free software: you can redistribute it and/or modify
 *   it under the terms of the GNU General Public License as published by
 *   the Free Software Foundation, either version 3 of the License, or
 *   (at your option) any later version.
 *
 *   This program is distributed in the hope that it will be useful,
 *   but WITHOUT ANY WARRANTY; without even the implied warranty of
 *   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 *   GNU General Public License for more details.
 *
 *   You should have received a copy of the GNU General Public License
 *   along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package main

import (
	"time"

	"github.com/sirupsen/logrus"
	"github.com/asdine/storm"
	"github.com/asdine/storm/q"
	"github.com/satori/go.uuid"

	"gopkg.in/telegram-bot-api.v4"
)

type bowlingDB struct {
	*storm.DB
	logger logrus.FieldLogger
}

type bowlingGroup struct {
	db        *bowlingDB
	ID        int64 `storm:"id"`
	Name      string
	Location  string
	Time      time.Time
	Day       time.Weekday
	Setup     bool
	NextEvent string
}

type bowlingUser struct {
	ID        int `storm:"id"`
	FirstName string
	LastName  string
	UserName  string
}

type bowlingEvent struct {
	db        *bowlingDB
	ID        string    `storm:"id"`
	Group     int64     `storm:"index"`
	Time      time.Time `storm:"index"`
	Attendees map[int]int
}

func dbOpen(path string) (*bowlingDB, error) {
	db, err := storm.Open(path)
	if err != nil {
		return nil, err
	}
	return &bowlingDB{DB: db}, nil
}

func (db *bowlingDB) log() logrus.FieldLogger {
	if db.logger == nil {
		db.logger = logrus.New()
	}
	return db.logger
}

func (db *bowlingDB) saveUser(tu *tgbotapi.User) {
	u := bowlingUser(*tu)
	err := db.Save(&u)
	if err != nil {
		db.log().WithError(err).Error("Unable to save user")
	}
}

func (db *bowlingDB) loadUser(id int) *tgbotapi.User {
	u := bowlingUser{}
	err := db.One("ID", id, &u)
	if err != nil {
		db.log().WithError(err).Error("Unable to load user")
	}
	tu := tgbotapi.User(u)
	return &tu
}

func (db *bowlingDB) getBowlingGroup(id int64) *bowlingGroup {
	log := db.log().WithField("id", id)
	g := new(bowlingGroup)
	err := db.One("ID", id, g)
	if err != nil {
		log.WithError(err).Warn("Unable to load bowling group")
	}
	g.ID = id
	g.db = db
	return g
}

func (db *bowlingDB) clean() {
	query := db.Select(q.Lt("Time", time.Now().Add(-24*time.Hour)))
	query.Delete(new(bowlingEvent))
}

func (g *bowlingGroup) save() {
	g.db.Save(g)
}

func (g *bowlingGroup) isConfigured() bool {
	return g.Setup
}

func (g *bowlingGroup) GetNextEvent() *bowlingEvent {
	if g.NextEvent == "" {
		return nil
	}
	e := new(bowlingEvent)
	if err := g.db.One("ID", g.NextEvent, e); err == nil {
		if e.Time.After(time.Now()) {
			e.db = g.db
			return e
		}
	}

	return nil
}

func (g *bowlingGroup) nextSlot() time.Time {
	days := 0
	day := int(g.Day)
	today := int(time.Now().Weekday())
	if today > day {
		days = day + 7 - today
	} else {
		days = day - today
	}
	return time.Now().Add(time.Duration(days*24) * time.Hour)
}

func (e *bowlingEvent) save() {
	e.db.log().WithField("event", e).Info("Saving event")
	e.db.Save(e)
}

func (g *bowlingGroup) NewEvent() *bowlingEvent {
	e := &bowlingEvent{
		ID:        uuid.NewV1().String(),
		Group:     g.ID,
		Time:      g.nextSlot(),
		Attendees: make(map[int]int),
	}
	g.NextEvent = e.ID
	g.save()
	e.db = g.db
	e.save()

	return e
}
