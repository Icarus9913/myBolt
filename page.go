package bolt

import (
	"fmt"
	"os"
	"sort"
	"unsafe"
)

const pageHeaderSize = int(unsafe.Offsetof(((*page)(nil)).ptr))		//复习一下内存对齐,这里是8+2+2+4=16个字节,表示在ptr之前的首部header占16个字节

const minKeysPerPage = 2

const branchPageElementSize = int(unsafe.Sizeof(branchPageElement{}))
const leafPageElementSize = int(unsafe.Sizeof(leafPageElement{}))

const (
	branchPageFlag   = 0x01		//1,树枝节点页
	leafPageFlag     = 0x02		//2,叶子节点页
	metaPageFlag     = 0x04		//4,元数据页
	freelistPageFlag = 0x10		//16,空闲列表页
)

const (
	bucketLeafFlag = 0x01		//1
)

type pgid uint64

/*
	page由两部分组成,
		1是page header: id,flags,count,overflow;
		2是page data: *ptr
*/
type page struct {
	id       pgid		//page id
	flags    uint16		//区分page类型的标识,可以为元数据,空闲列表,树枝,叶子 这四种之一
	count    uint16		//记录page中的元素个数
	overflow uint32		//当遇到体积巨大、单个 page 无法装下的数据时，会溢出到其它 pages，overflow 记录溢出数
	ptr      uintptr	//指向 page 数据的内存地址，该字段仅在内存中存在,并不落盘. 可指向meta,branch,leaf,freelist
}

// typ returns a human readable page type string used for debugging.
func (p *page) typ() string {
	if (p.flags & branchPageFlag) != 0 {
		return "branch"
	} else if (p.flags & leafPageFlag) != 0 {
		return "leaf"
	} else if (p.flags & metaPageFlag) != 0 {
		return "meta"
	} else if (p.flags & freelistPageFlag) != 0 {
		return "freelist"
	}
	return fmt.Sprintf("unknown<%02x>", p.flags)
}

// meta returns a pointer to the metadata section of the page.
func (p *page) meta() *meta {
	return (*meta)(unsafe.Pointer(&p.ptr))
}

// leafPageElement retrieves the leaf node by index
func (p *page) leafPageElement(index uint16) *leafPageElement {
	n := &((*[0x7FFFFFF]leafPageElement)(unsafe.Pointer(&p.ptr)))[index]
	return n
}

// leafPageElements retrieves a list of leaf nodes.
func (p *page) leafPageElements() []leafPageElement {
	if p.count == 0 {
		return nil
	}
	return ((*[0x7FFFFFF]leafPageElement)(unsafe.Pointer(&p.ptr)))[:]
}

// branchPageElement retrieves the branch node by index
func (p *page) branchPageElement(index uint16) *branchPageElement {
	return &((*[0x7FFFFFF]branchPageElement)(unsafe.Pointer(&p.ptr)))[index]
}

// branchPageElements retrieves a list of branch nodes.
func (p *page) branchPageElements() []branchPageElement {
	if p.count == 0 {
		return nil
	}
	return ((*[0x7FFFFFF]branchPageElement)(unsafe.Pointer(&p.ptr)))[:]
}

// dump writes n bytes of the page to STDERR as hex output.
func (p *page) hexdump(n int) {
	buf := (*[maxAllocSize]byte)(unsafe.Pointer(p))[:n]
	fmt.Fprintf(os.Stderr, "%x\n", buf)
}

type pages []*page

func (s pages) Len() int           { return len(s) }
func (s pages) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s pages) Less(i, j int) bool { return s[i].id < s[j].id }

// branchPageElement represents a node on a branch page.
// 树分支
type branchPageElement struct {
	pos   uint32	//键的位置,即位移			key的位置
	ksize uint32	//键的长度				key的大小
	pgid  pgid		//子节点所在page的id		指向下层pageid
}

// key returns a byte slice of the node key.
func (n *branchPageElement) key() []byte {
	buf := (*[maxAllocSize]byte)(unsafe.Pointer(n))		// buf 是元素头起始地址
	return (*[maxAllocSize]byte)(unsafe.Pointer(&buf[n.pos]))[:n.ksize]
}

// leafPageElement represents a node on a leaf page.
// 叶子节点
type leafPageElement struct {
	flags uint32	//保留字段，同时方便对齐	/内容标识,可以为普通数据(0)或bucket(1)两者中的任一种
	pos   uint32	//键值的位置，即位移		key的位置
	ksize uint32	//键的长度				key的大小
	vsize uint32	//值的长度				value的大小
}

// key returns a byte slice of the node key.
func (n *leafPageElement) key() []byte {
	buf := (*[maxAllocSize]byte)(unsafe.Pointer(n))
	return (*[maxAllocSize]byte)(unsafe.Pointer(&buf[n.pos]))[:n.ksize:n.ksize]
}

// value returns a byte slice of the node value.
func (n *leafPageElement) value() []byte {
	buf := (*[maxAllocSize]byte)(unsafe.Pointer(n))
	return (*[maxAllocSize]byte)(unsafe.Pointer(&buf[n.pos+n.ksize]))[:n.vsize:n.vsize]
}

// PageInfo represents human readable information about a page.
type PageInfo struct {
	ID            int
	Type          string
	Count         int
	OverflowCount int
}

type pgids []pgid

func (s pgids) Len() int           { return len(s) }
func (s pgids) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s pgids) Less(i, j int) bool { return s[i] < s[j] }

// merge returns the sorted union of a and b.
func (a pgids) merge(b pgids) pgids {
	// Return the opposite slice if one is nil.
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	merged := make(pgids, len(a)+len(b))
	mergepgids(merged, a, b)
	return merged
}

// mergepgids copies the sorted union of a and b into dst.
// If dst is too small, it panics.
func mergepgids(dst, a, b pgids) {
	if len(dst) < len(a)+len(b) {
		panic(fmt.Errorf("mergepgids bad len %d < %d + %d", len(dst), len(a), len(b)))
	}
	// Copy in the opposite slice if one is nil.
	if len(a) == 0 {
		copy(dst, b)
		return
	}
	if len(b) == 0 {
		copy(dst, a)
		return
	}

	// Merged will hold all elements from both lists.
	merged := dst[:0]

	// Assign lead to the slice with a lower starting value, follow to the higher value.
	lead, follow := a, b
	if b[0] < a[0] {
		lead, follow = b, a
	}

	// Continue while there are elements in the lead.
	for len(lead) > 0 {
		// Merge largest prefix of lead that is ahead of follow[0].
		n := sort.Search(len(lead), func(i int) bool { return lead[i] > follow[0] })
		merged = append(merged, lead[:n]...)
		if n >= len(lead) {
			break
		}

		// Swap lead and follow.
		lead, follow = follow, lead[n:]
	}

	// Append what's left in follow.
	_ = append(merged, follow...)
}
