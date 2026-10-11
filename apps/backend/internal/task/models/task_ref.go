package models

// TaskRef is the minimal identity of a task shown as a link.
type TaskRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}
