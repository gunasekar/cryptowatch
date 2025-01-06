package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

type Service interface {
	SendNotification(title, body string) error
}

type MultiNotifier struct {
	services []Service
}

func NewMultiNotifier(services []Service) *MultiNotifier {
	return &MultiNotifier{
		services: services,
	}
}

func (m *MultiNotifier) SendNotification(title, body string) error {
	var errs []string
	var wg sync.WaitGroup
	errChan := make(chan error, len(m.services))

	for _, service := range m.services {
		wg.Add(1)
		go func(s Service) {
			defer wg.Done()
			if err := s.SendNotification(title, body); err != nil {
				errChan <- err
			}
		}(service)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return fmt.Errorf("notification errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

type PushbulletNotifier struct {
	token string
}

func NewPushbulletNotifier(token string) *PushbulletNotifier {
	return &PushbulletNotifier{
		token: token,
	}
}

type PushbulletRequest struct {
	Body  string `json:"body"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

func (p *PushbulletNotifier) SendNotification(title, body string) error {
	pushData := PushbulletRequest{
		Body:  body,
		Title: title,
		Type:  "note",
	}

	jsonData, err := json.Marshal(pushData)
	if err != nil {
		return fmt.Errorf("error marshaling push notification: %v", err)
	}

	req, err := http.NewRequest("POST", "https://api.pushbullet.com/v2/pushes", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating push request: %v", err)
	}

	req.Header.Set("Access-Token", p.token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending push notification: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("push notification failed with status: %d", resp.StatusCode)
	}

	return nil
}
