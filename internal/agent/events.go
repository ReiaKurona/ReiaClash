package agent

import "time"

type Event struct {
	ID      uint64    `json:"id"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
	At      time.Time `json:"at"`
}

func (a *App) RequestExit() {
	a.exitOnce.Do(func() { close(a.Exit) })
}

func (a *App) AddEvent(level, message string) {
	a.eventsMu.Lock()
	defer a.eventsMu.Unlock()
	a.eventID++
	a.events = append(a.events, Event{ID: a.eventID, Level: level, Message: message, At: time.Now().UTC()})
	if len(a.events) > 100 {
		a.events = append([]Event(nil), a.events[len(a.events)-100:]...)
	}
}

func (a *App) EventsAfter(id uint64) []Event {
	a.eventsMu.Lock()
	defer a.eventsMu.Unlock()
	var result []Event
	for _, event := range a.events {
		if event.ID > id {
			result = append(result, event)
		}
	}
	return result
}
