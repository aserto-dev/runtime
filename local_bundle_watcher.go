package runtime

import (
	"context"
	"time"

	"github.com/aserto-dev/runtime/internal/pathwatcher"
	initload "github.com/aserto-dev/runtime/internal/runtime/init"
	"github.com/fsnotify/fsnotify"
	"github.com/open-policy-agent/opa/v1/storage"
)

func (r *Runtime) onReloadLogger(d time.Duration, err error) {
	r.Logger.Warn().
		Dur("duration", d).
		Err(err).
		Msg("Processed file watch event.")
}

func (r *Runtime) startWatcher(ctx context.Context, paths []string, onReload func(time.Duration, error)) error {
	watcher, err := r.getWatcher(paths)
	if err != nil {
		return err
	}

	go r.readWatcher(ctx, watcher, paths, onReload)

	return nil
}

func (rt *Runtime) readWatcher(ctx context.Context, watcher *fsnotify.Watcher, paths []string, onReload func(time.Duration, error)) {
	for {
		select {
		case evt := <-watcher.Events:
			removalMask := fsnotify.Remove | fsnotify.Rename
			mask := fsnotify.Create | fsnotify.Write | removalMask

			if (evt.Op & mask) != 0 {
				rt.Logger.Debug().Str("event", evt.String()).Msg("Registered file event")

				t0 := time.Now()
				removed := ""

				if (evt.Op & removalMask) != 0 {
					removed = evt.Name
				}

				err := rt.processWatcherUpdate(ctx, paths, removed)
				onReload(time.Since(t0), err)
			}
		case <-ctx.Done():
			_ = watcher.Close()
			return
		}
	}
}

func (rt *Runtime) getWatcher(rootPaths []string) (*fsnotify.Watcher, error) {
	watcher, err := pathwatcher.CreatePathWatcher(rootPaths)
	if err != nil {
		return nil, err
	}

	for _, path := range watcher.WatchList() {
		rt.Logger.Debug().Str("path", path).Msg("watching path")
	}

	return watcher, nil
}

func (rt *Runtime) processWatcherUpdate(ctx context.Context, paths []string, removed string) error {
	return pathwatcher.ProcessWatcherUpdateForRegoVersion(
		ctx,
		rt.RegoVersion(),
		paths,
		removed,
		rt.Store(),
		rt.Params.Filter,
		rt.Params.BundleMode,
		rt.Params.BundleLazyLoadingMode,
		func(ctx context.Context, txn storage.Transaction, loaded *initload.LoadPathsResult) error {
			_, err := initload.InsertAndCompile(ctx, initload.InsertAndCompileOptions{
				Store:         rt.Store(),
				Txn:           txn,
				Files:         loaded.Files,
				Bundles:       loaded.Bundles,
				MaxErrors:     -1,
				ParserOptions: rt.GetPluginsManager().ParserOptions(),
			})

			return err
		})
}
