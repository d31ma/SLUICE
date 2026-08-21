package appui

import (
	"context"
	"sync"
)

type Agent interface {
	Start(ctx context.Context) error
	Close(ctx context.Context) error
}

type Shell interface {
	Show()
	Hide()
	Quit()
	Notify(title, content string)
}

type Session struct {
	agent    Agent
	shell    Shell
	quitErr  error
	quitOnce sync.Once
}

func NewSession(agent Agent, shell Shell) *Session {
	return &Session{agent: agent, shell: shell}
}

func (session *Session) Start(ctx context.Context) error {
	if err := session.agent.Start(ctx); err != nil {
		return err
	}
	session.shell.Show()
	return nil
}

func (session *Session) Show() {
	session.shell.Show()
}

func (session *Session) Hide() {
	session.shell.Hide()
}

func (session *Session) Notify(title, content string) {
	session.shell.Notify(title, content)
}

func (session *Session) Quit(ctx context.Context) error {
	session.quitOnce.Do(func() {
		session.quitErr = session.agent.Close(ctx)
		session.shell.Quit()
	})
	return session.quitErr
}
