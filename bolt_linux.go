package bolt

import (
	"syscall"
)

// fdatasync flushes written data to a file descriptor.
// 将数据从内存刷入磁盘
func fdatasync(db *DB) error {
	return syscall.Fdatasync(int(db.file.Fd()))
}
