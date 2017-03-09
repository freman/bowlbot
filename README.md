# BowlBot

Organise bowling games with a Telegram bot

# Commands

## /bowling

Schedule a game for the next day one would be scheduled

## /me

Say you're coming, use +1 to bring an extra, or +2 for 2. etc

## /sub count

Say one of your extras aren't coming

## /out

Say you're no longer coming

## /count

How many are coming

## /who

List of people coming

# Environment Variables

## API_KEY

The apikey from Botfather

## CALLBACK_URL

The URL for telegram to call, must be reachable

## STATE_DB

Path to the storage for bot state

default: `state.db`

## LISTEN

The host:port combination to listen on

default: `127.0.0.1:8000`

## License

Copyright (c) 2017 Shannon Wynter. Licensed under GPL3. See the [LICENSE.md](LICENSE.md) file for a copy of the license.
