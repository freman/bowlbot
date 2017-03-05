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
	"fmt"
	"sync"
	"time"

	"gopkg.in/telegram-bot-api.v4"
)

type convoNext struct {
	c time.Time
	f func(message *tgbotapi.Message)
}

type convoState struct {
	sync.Mutex
	inReplyTo map[int]convoNext
}

var weekDays = []time.Weekday{time.Sunday, time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday}
var weekDayMap = make(map[string]time.Weekday)
var weekdayKeyboard tgbotapi.ReplyKeyboardMarkup

func init() {
	var weekDayKeyboardButtons = make([]tgbotapi.KeyboardButton, 7)
	for n, v := range weekDays {
		weekDayKeyboardButtons[n] = tgbotapi.NewKeyboardButton(v.String())
		weekDayMap[v.String()] = v
	}
	weekdayKeyboard := tgbotapi.NewReplyKeyboard(weekDayKeyboardButtons)
	weekdayKeyboard.OneTimeKeyboard = true
	weekdayKeyboard.Selective = true
	weekdayKeyboard.ResizeKeyboard = true
}

func (c *convoState) setNext(id int, f func(message *tgbotapi.Message)) {
	c.Lock()
	c.inReplyTo[id] = convoNext{c: time.Now(), f: f}
	c.Unlock()
}

func (c *convoState) next(message *tgbotapi.Message) {
	if message.ReplyToMessage == nil {
		return
	}
	id := message.ReplyToMessage.MessageID
	c.Lock()
	n := c.inReplyTo[id]
	delete(c.inReplyTo, id)
	c.Unlock()
	n.f(message)
}

func (c *convoState) maintain() {
	for {
		time.Sleep(5 * time.Minute)
		deadline := time.Now().Add(-10 * time.Minute)
		c.Lock()
		for id, n := range c.inReplyTo {
			if n.c.Before(deadline) {
				delete(c.inReplyTo, id)
			}
		}
		c.Unlock()
	}
}

func newConvoState() *convoState {
	cs := &convoState{inReplyTo: make(map[int]convoNext)}
	go cs.maintain()
	return cs
}

func (b *bowlBot) doneAsking(message *tgbotapi.Message) {
	chat := message.Chat
	group := b.db.getBowlingGroup(chat.ID)

	t, err := time.Parse("15:04", message.Text)
	if err != nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, "I'm sorry, what?")
		msg.ReplyToMessageID = message.MessageID
		msg.DisableNotification = true
		msg.ReplyMarkup = tgbotapi.ForceReply{
			Selective:  true,
			ForceReply: true,
		}
		sentMessage, err := b.bot.Send(msg)
		if err != nil {
			b.log().WithError(err).Warn("Unable to send message")
		}
		b.convos.setNext(sentMessage.MessageID, b.doneAsking)
		return
	}
	group.Time = t
	group.Setup = true
	group.save()

	msg := tgbotapi.NewMessage(message.Chat.ID, "Awesome, all done")
	msg.ReplyToMessageID = message.MessageID
	msg.DisableNotification = true

	b.bot.Send(msg)
}

func (b *bowlBot) askForTime(message *tgbotapi.Message) {
	chat := message.Chat
	group := b.db.getBowlingGroup(chat.ID)
	var msg tgbotapi.MessageConfig
	var nextReply func(message *tgbotapi.Message)
	if day, found := weekDayMap[message.Text]; found {
		group.Day = day
		group.save()

		msg = tgbotapi.NewMessage(message.Chat.ID, "And what time does it start?")
		msg.ReplyMarkup = tgbotapi.ForceReply{
			Selective:  true,
			ForceReply: true,
		}
		nextReply = b.doneAsking
	} else {
		msg = tgbotapi.NewMessage(message.Chat.ID, "I'm sorry, what?")
		msg.ReplyMarkup = weekdayKeyboard
		nextReply = b.askForTime
	}

	msg.ReplyToMessageID = message.MessageID
	msg.DisableNotification = true

	sentMessage, err := b.bot.Send(msg)
	if err != nil {
		b.log().WithError(err).Warn("Unable to send message")
	}

	b.convos.setNext(sentMessage.MessageID, nextReply)
}

func (b *bowlBot) askForDayOfWeek(message *tgbotapi.Message) {
	chat := message.Chat
	group := b.db.getBowlingGroup(chat.ID)
	group.Location = message.Text
	group.save()

	msg := tgbotapi.NewMessage(message.Chat.ID, "And what day do you do this?")
	msg.ReplyToMessageID = message.MessageID

	msg.DisableNotification = true

	msg.ReplyMarkup = weekdayKeyboard
	sentMessage, err := b.bot.Send(msg)
	if err != nil {
		b.log().WithError(err).Warn("Unable to send message")
	}

	b.convos.setNext(sentMessage.MessageID, b.askForTime)
}

func (b *bowlBot) askForConfiguration(message *tgbotapi.Message) {
	msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Hi %s, your group isn't set up for bowling yet. Please answer the following questions.", message.From.String()))
	b.bot.Send(msg)
	msg = tgbotapi.NewMessage(message.Chat.ID, "Where is it that you go bowling?")
	msg.ReplyToMessageID = message.MessageID
	msg.ReplyMarkup = tgbotapi.ForceReply{
		Selective:  true,
		ForceReply: true,
	}
	msg.DisableNotification = true

	sentMessage, err := b.bot.Send(msg)
	if err != nil {
		b.log().WithError(err).Warn("Unable to send message")
	}
	b.convos.setNext(sentMessage.MessageID, b.askForDayOfWeek)
}
