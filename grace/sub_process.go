/*
 * Copyright(C) 2021 github.com/hidu  All Rights Reserved.
 * Author: hidu (duv123+git@baidu.com)
 * Date: 2021/1/12
 */

package grace

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

// subProcess 子进程的逻辑
type subProcess struct {
	shutDownTimeout time.Duration
	resources       []Resource
	Log             *log.Logger
}

func (sp *subProcess) logit(msgs ...interface{}) {
	msg := fmt.Sprintf("[grace][sub_process] pid=%d ppid=%d %s", os.Getpid(), os.Getppid(), fmt.Sprint(msgs...))
	sp.Log.Output(1, msg)
}

func (sp *subProcess) Start(ctx context.Context) (errLast error) {
	sp.logit("Start() start")
	start := time.Now()
	defer func() {
		cost := time.Since(start)
		sp.logit("Start() finish, error=", errLast,
			", start_at=", start.Format("2006-01-02 15:04:05"),
			", duration=", cost,
		)
	}()

	var errChan chan error
	for idx, s := range sp.resources {
		f := os.NewFile(uintptr(3+idx), "")
		s.SetFile(f)

		go func(res Resource) {
			errChan <- res.Start(ctx)
		}(s)
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	var err error

	select {
	case <-ctx.Done():
		err = ctx.Err()
	case e1 := <-errChan:
		err = e1
	case sig := <-ch:
		err = fmt.Errorf("exit by signal(%v)", sig)
	}

	ctx, cancel := context.WithTimeout(ctx, sp.shutDownTimeout)
	defer cancel()

	sp.Stop(ctx)

	return err
}

func (sp *subProcess) Stop(ctx context.Context) (errStop error) {
	sp.logit("Stop() start")
	defer func() {
		sp.logit("Stop() finish, error=", errStop)
	}()

	var wg sync.WaitGroup
	errChans := make(chan error, len(sp.resources))
	for idx, s := range sp.resources {
		wg.Add(1)

		go func(idx int, res Resource) {
			defer wg.Done()

			if err := res.Stop(ctx); err != nil {
				errChans <- fmt.Errorf("resource[%d] (%) Stop error: %w", idx, res.String(), err)
			} else {
				errChans <- nil
			}
		}(idx, s)
	}
	wg.Wait()

	close(errChans)

	var bd strings.Builder
	for err := range errChans {
		if err != nil {
			bd.WriteString(err.Error())
			bd.WriteString(";")
		}
	}
	if bd.Len() == 0 {
		return nil
	}
	return errors.New(bd.String())
}
