package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"golang.org/x/net/webdav"
)

var (
	multiSlashRegexp = regexp.MustCompile(`/{2,}`)
	listCacheTime    = 1 * time.Minute
	fileCacheTime    = 1 * time.Minute
	itemCacheTime    = 1 * time.Minute
)

type fileStat struct {
	f *DriveItem
}

func (f *fileStat) Name() string {
	return f.f.Name
}

func (f *fileStat) Size() int64 {
	i, _ := strconv.Atoi(f.f.Size)
	return int64(i)
}

func (f *fileStat) Mode() os.FileMode {
	if f.f.IsFolder() {
		return os.ModeDir | 0777
	}
	return 0444
}

func (f *fileStat) ModTime() time.Time {
	t, err := time.Parse(time.RFC3339, f.f.ModifiedTime)
	if err != nil {
		return time.Unix(0, 0)
	}
	return t.UTC()
}

func (f *fileStat) IsDir() bool {
	return f.f.IsFolder()
}

func (f *fileStat) Sys() interface{} {
	return nil
}

type FileSystem struct {
	c         *Client
	itemCache *ttlcache.Cache[string, *DriveItem]
	listCache *ttlcache.Cache[string, *DriveFileList]
	fileCache *ttlcache.Cache[string, *DriveFile]
	mutex     sync.RWMutex
}

func (d *FileSystem) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	return os.ErrPermission
}

func (d *FileSystem) cachedList(ctx context.Context, item *DriveItem) (*DriveFileList, error) {
	var err error
	var dir *DriveFileList
	cached := d.listCache.Get(item.ID)
	if cached != nil {
		dir = cached.Value()
	} else {
		dir, err = item.List(ctx)
		if err != nil {
			return nil, err
		}
		d.listCache.Set(item.ID, dir, listCacheTime)
	}
	return dir, nil
}

func (d *FileSystem) cachedFetch(ctx context.Context, item *DriveItem) (*DriveFile, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	var err error
	var file *DriveFile
	cached := d.fileCache.Get(item.ID)
	if cached != nil {
		file = cached.Value()
	} else {
		file, err = item.Fetch(ctx)
		if err != nil {
			return nil, err
		}
		d.fileCache.Set(item.ID, file, fileCacheTime)
	}
	return file, nil
}

func (d *FileSystem) driveItemToFile(ctx context.Context, item *DriveItem) (*File, error) {
	if item == nil {
		return nil, os.ErrNotExist
	}
	fctx, cancel := context.WithCancel(ctx)
	return &File{
		ctx:    fctx,
		cancel: cancel,
		fs:     d,
		stat: &fileStat{
			f: item,
		},
	}, nil
}

func (d *FileSystem) walkTo(ctx context.Context, target string, curPath string, curItem *DriveItem) (*DriveItem, error) {
	d.itemCache.Set(curPath, curItem, itemCacheTime)

	if target == curPath {
		return curItem, nil
	}

	if !strings.HasPrefix(target, curPath) {
		return nil, fmt.Errorf("target %s is not a child of %s", target, curPath)
	}

	if !curItem.IsFolder() {
		return nil, fmt.Errorf("current path %s not a folder", curPath)
	}

	rest := strings.Trim(target[len(curPath):], "/")
	next := strings.SplitN(rest, "/", 2)[0]
	nextPath := curPath + "/" + next

	var nextItem *DriveItem
	cachedNextItem := d.itemCache.Get(nextPath)
	if cachedNextItem != nil {
		nextItem = cachedNextItem.Value()
	} else {
		dir, err := d.cachedList(ctx, curItem)
		if err != nil {
			return nil, err
		}
		nextItem = dir.Get(next)
	}
	if nextItem == nil {
		d.itemCache.Set(nextPath, nil, itemCacheTime)
		return nil, nil
	}

	return d.walkTo(ctx, target, nextPath, nextItem)
}

func sanitizeName(name string) string {
	name = strings.TrimPrefix(name, "./")
	name = strings.TrimSuffix(name, "/")
	name = multiSlashRegexp.ReplaceAllString(name, "/")
	return name
}

func (d *FileSystem) getDriveItem(ctx context.Context, name string) (*DriveItem, error) {
	name = sanitizeName(name)

	var err error
	var item *DriveItem

	root, err := d.c.Root()
	if err != nil {
		return nil, err
	}

	cachedItem := d.itemCache.Get(name)
	if cachedItem != nil {
		item = cachedItem.Value()
	} else {
		item, err = d.walkTo(ctx, name, "", root)
		if err != nil {
			return nil, err
		}
	}

	return item, nil
}

func (d *FileSystem) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	item, err := d.getDriveItem(ctx, name)
	if err != nil {
		return nil, err
	}

	return d.driveItemToFile(ctx, item)
}

func (d *FileSystem) Open(name string) (http.File, error) {
	return d.OpenFile(context.Background(), name, os.O_RDONLY, 0)
}

func (d *FileSystem) RemoveAll(ctx context.Context, name string) error {
	name = sanitizeName(name)

	if name == "" {
		// don't allow deleting root
		return os.ErrPermission
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()

	item, err := d.getDriveItem(ctx, name)

	if err != nil {
		return err
	}

	if item == nil {
		return os.ErrNotExist
	}

	var expiredKeys []string
	keys := d.itemCache.Keys()
	for _, key := range keys {
		if strings.HasPrefix(key, name) {
			expiredKeys = append(expiredKeys, key)
		}
	}
	for _, key := range expiredKeys {
		d.itemCache.Delete(key)
	}

	d.listCache.Delete(item.ID)
	d.fileCache.Delete(item.ID)

	return item.Trash(ctx)
}

func (d *FileSystem) Rename(ctx context.Context, oldname, newname string) error {
	return os.ErrPermission
}

func (d *FileSystem) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	item, err := d.getDriveItem(ctx, name)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, os.ErrNotExist
	}

	return &fileStat{f: item}, nil
}

type File struct {
	fs     *FileSystem
	ctx    context.Context
	cancel context.CancelFunc

	rc io.ReadCloser

	fPos int64
	dPos int

	mutex sync.Mutex
	stat  *fileStat
}

func (f *File) Readdir(count int) (fs []os.FileInfo, err error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if !f.stat.IsDir() {
		return nil, os.ErrInvalid
	}

	dir, err := f.fs.cachedList(f.ctx, f.stat.f)
	if err != nil {
		return nil, err
	}

	if count <= 0 || count > len(dir.Files)-f.dPos {
		count = len(dir.Files) - f.dPos
	}

	for i := 0; i < count; i++ {
		fs = append(fs, &fileStat{f: dir.Files[f.dPos]})
		f.dPos++
	}

	return fs, nil
}

func (f *File) Stat() (os.FileInfo, error) {
	return f.stat, nil
}

func (f *File) Read(b []byte) (n int, err error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.stat.IsDir() {
		return 0, os.ErrInvalid
	}

	size := f.stat.Size()
	if f.fPos >= size {
		f.rc.Close()
		f.rc = nil
		return 0, io.EOF
	}

	if f.rc == nil {
		file, err := f.fs.cachedFetch(f.ctx, f.stat.f)
		if err != nil {
			return 0, err
		}

		req, err := http.NewRequestWithContext(f.ctx, http.MethodGet, file.WebContentLink, nil)
		if err != nil {
			return 0, err
		}
		req.Header = map[string][]string{
			"Range": {"bytes=" + fmt.Sprint(f.fPos) + "-" + fmt.Sprint(size-1)},
		}
		resp, err := f.fs.c.downloadHTTPClient.Do(req)
		if err != nil {
			return 0, err
		}

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			return 0, fmt.Errorf("unexpected status code %d", resp.StatusCode)
		}

		f.rc = resp.Body
	}

	n, err = io.ReadFull(f.rc, b)
	f.fPos += int64(n)

	return n, err
}

func (f *File) Seek(offset int64, whence int) (int64, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.stat.IsDir() {
		return 0, os.ErrInvalid
	}

	npos := f.fPos
	size := f.stat.Size()

	switch whence {
	case io.SeekStart:
		npos = offset
	case io.SeekCurrent:
		npos += offset
	case io.SeekEnd:
		npos = size + offset
	default:
		npos = -1
	}

	if npos < 0 {
		return 0, os.ErrInvalid
	}

	if npos == f.fPos {
		return npos, nil
	}

	f.fPos = npos

	if f.rc != nil {
		err := f.rc.Close()
		f.rc = nil
		if err != nil {
			return 0, err
		}
	}

	return f.fPos, nil
}

func (f *File) Close() error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.cancel != nil {
		defer f.cancel()
	}

	if f.rc != nil {
		return f.rc.Close()
	}

	return nil
}

func (f *File) Write(b []byte) (n int, err error) {
	return 0, os.ErrPermission
}

func (c *Client) FileSystem() (*FileSystem, error) {
	return &FileSystem{
		c:         c,
		itemCache: ttlcache.New[string, *DriveItem](),
		listCache: ttlcache.New[string, *DriveFileList](),
		fileCache: ttlcache.New[string, *DriveFile](),
	}, nil
}
