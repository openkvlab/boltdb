package freelist

import (
	"fmt"
	"sort"
	"unsafe"

	"github.com/openkvlab/boltdb/internal/common"
)

type txPending struct {
	ids     []common.Pgid
	alloctx []common.Txid // txids allocating the page ids
}

type shared struct {
	Interface

	readonlyTXIDs []common.Txid               // all readonly transaction IDs.
	allocs        map[common.Pgid]common.Txid // mapping of Txid that allocated a pgid.
	cache         map[common.Pgid]struct{}    // fast lookup of all free and pending page ids.
	pending       map[common.Txid]*txPending  // mapping of soon-to-be free page ids by tx.
}

func newShared() *shared {
	return &shared{
		pending: make(map[common.Txid]*txPending),
		allocs:  make(map[common.Pgid]common.Txid),
		cache:   make(map[common.Pgid]struct{}),
	}
}

func (t *shared) pendingPageIds() map[common.Txid]*txPending {
	return t.pending
}

func (t *shared) PendingCount() int {
	var count int
	for _, txp := range t.pending {
		count += len(txp.ids)
	}
	return count
}

func (t *shared) Count() int {
	return t.FreeCount() + t.PendingCount()
}

func (t *shared) Freed(pgId common.Pgid) bool {
	_, ok := t.cache[pgId]
	return ok
}

func (t *shared) Free(txid common.Txid, p *common.Page) {
	if p.Id() <= 1 {
		panic(fmt.Sprintf("cannot free page 0 or 1: %d", p.Id()))
	}

	// Free page and all its overflow pages.
	txp := t.pending[txid]
	if txp == nil {
		txp = &txPending{}
		t.pending[txid] = txp
	}
	allocTxid, ok := t.allocs[p.Id()]
	common.Verify(func() {
		// When a writing transaction frees a page, that page must have been
		// allocated by one of previous committed transactions.
		//
		// Pages allocated during the current transaction can only come from
		// two sources:
		//   1. The committed freelist
		//   2. Extending the database file (by remapping it to a larger size)
		//
		// If the current transaction fails to commit, we do **not** need to
		// return these newly allocated pages to the freelist:
		//   • For pages taken from the freelist: on rollback, we simply reload
		//     the freelist from the last committed state — the pages automatically
		//     become free again.
		//   • For pages obtained by growing the file: since the meta page was
		//     never updated, the new region remains invisible to all transactions
		//     (including future write transactions). The space is not lost — it
		//     will be reused the next time the database actually needs to grow.
		//
		// Additionally, before the current transaction commits, none of its
		// newly allocated pages are visible to any concurrent read-only
		// transactions, preserving full isolation (repeatable read).
		if allocTxid == txid {
			panic(fmt.Sprintf("free: freed page (%d) was allocated by the same transaction (%d)", p.Id(), txid))
		}
	})
	if ok {
		delete(t.allocs, p.Id())
	}

	for id := p.Id(); id <= p.Id()+common.Pgid(p.Overflow()); id++ {
		// Verify that page is not already free.
		if _, ok := t.cache[id]; ok {
			panic(fmt.Sprintf("page %d already freed", id))
		}
		// Add to the freelist and cache.
		txp.ids = append(txp.ids, id)
		txp.alloctx = append(txp.alloctx, allocTxid)
		t.cache[id] = struct{}{}
	}
}

func (t *shared) Rollback(txid common.Txid) {
	// Remove page ids from cache.
	txp := t.pending[txid]
	if txp == nil {
		return
	}
	for i, pgid := range txp.ids {
		delete(t.cache, pgid)
		tx := txp.alloctx[i]
		if tx == 0 {
			continue
		}
		if tx != txid {
			// Pending free aborted; restore page back to alloc list.
			t.allocs[pgid] = tx
		} else {
			// Since a writing transaction will never free a page which was
			// allocated by itself, so when rollback, for all the pages which
			// were freed by current transaction, they must have been allocated
			// by previous transactions. It's impossible that they are allocated
			// by current transaction.
			panic(fmt.Sprintf("rollback: freed page (%d) was allocated by the same transaction (%d)", pgid, txid))
		}
	}
	// Remove pages from pending list and mark as free if allocated by txid.
	delete(t.pending, txid)

	// Remove pgids which are allocated by this txid
	for pgid, tid := range t.allocs {
		if tid == txid {
			delete(t.allocs, pgid)
		}
	}
}

func (t *shared) AddReadonlyTXID(tid common.Txid) {
	t.readonlyTXIDs = append(t.readonlyTXIDs, tid)
}

func (t *shared) RemoveReadonlyTXID(tid common.Txid) {
	for i := range t.readonlyTXIDs {
		if t.readonlyTXIDs[i] == tid {
			last := len(t.readonlyTXIDs) - 1
			t.readonlyTXIDs[i] = t.readonlyTXIDs[last]
			t.readonlyTXIDs = t.readonlyTXIDs[:last]
			break
		}
	}
}

type txIDx []common.Txid

func (t txIDx) Len() int           { return len(t) }
func (t txIDx) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t txIDx) Less(i, j int) bool { return t[i] < t[j] }

func (t *shared) ReleasePendingPages() {
	var m common.Pgids
	for ftxid, txp := range t.pending {
		for i := 0; i < len(txp.ids); i++ {
			atxid := txp.alloctx[i]

			safe2Release := true
			for _, rtxid := range t.readonlyTXIDs {
				// If a free page is visible to any readonly TXN, then we
				// can't completely release the page.
				if atxid <= rtxid && rtxid < ftxid {
					safe2Release = false
					break
				}
			}

			if safe2Release {
				m = append(m, txp.ids[i])
				txp.ids[i] = txp.ids[len(txp.ids)-1]
				txp.ids = txp.ids[:len(txp.ids)-1]
				txp.alloctx[i] = txp.alloctx[len(txp.alloctx)-1]
				txp.alloctx = txp.alloctx[:len(txp.alloctx)-1]
				i--
			}
		}

		if len(txp.ids) == 0 {
			delete(t.pending, ftxid)
		}
	}

	t.mergeSpans(m)
}

// Copyall copies a list of all free ids and all pending ids in one sorted list.
// f.count returns the minimum length required for dst.
func (t *shared) Copyall(dst []common.Pgid) {
	m := make(common.Pgids, 0, t.PendingCount())
	for _, txp := range t.pendingPageIds() {
		m = append(m, txp.ids...)
	}
	sort.Sort(m)
	common.Mergepgids(dst, t.freePageIds(), m)
}

func (t *shared) Reload(p *common.Page) {
	t.Read(p)
	t.NoSyncReload(t.freePageIds())
}

func (t *shared) NoSyncReload(pgIds common.Pgids) {
	// Build a cache of only pending pages.
	pcache := make(map[common.Pgid]struct{})
	for _, txp := range t.pending {
		for _, pendingID := range txp.ids {
			pcache[pendingID] = struct{}{}
		}
	}

	// Check each page in the freelist and build a new available freelist
	// with any pages not in the pending lists.
	a := []common.Pgid{}
	for _, id := range pgIds {
		if _, ok := pcache[id]; !ok {
			a = append(a, id)
		}
	}

	t.Init(a)
}

// reindex rebuilds the free cache based on available and pending free lists.
func (t *shared) reindex() {
	free := t.freePageIds()
	pending := t.pendingPageIds()
	t.cache = make(map[common.Pgid]struct{}, len(free))
	for _, id := range free {
		t.cache[id] = struct{}{}
	}
	for _, txp := range pending {
		for _, pendingID := range txp.ids {
			t.cache[pendingID] = struct{}{}
		}
	}
}

func (t *shared) Read(p *common.Page) {
	if !p.IsFreelistPage() {
		panic(fmt.Sprintf("invalid freelist page: %d, page type is %s", p.Id(), p.Typ()))
	}

	ids := p.FreelistPageIds()

	// Copy the list of page ids from the freelist.
	if len(ids) == 0 {
		t.Init([]common.Pgid{})
	} else {
		// copy the ids, so we don't modify on the freelist page directly
		idsCopy := make([]common.Pgid, len(ids))
		copy(idsCopy, ids)
		// Make sure they're sorted.
		sort.Sort(common.Pgids(idsCopy))

		t.Init(idsCopy)
	}
}

func (t *shared) EstimatedWritePageSize() int {
	n := t.Count()
	if n >= 0xFFFF {
		// The first element will be used to store the count. See freelist.write.
		n++
	}
	return int(common.PageHeaderSize) + (int(unsafe.Sizeof(common.Pgid(0))) * n)
}

func (t *shared) Write(p *common.Page) {
	// Combine the old free pgids and pgids waiting on an open transaction.

	// Update the header flag.
	p.SetFlags(common.FreelistPageFlag)

	// The page.count can only hold up to 64k elements so if we overflow that
	// number then we handle it by putting the size in the first element.
	l := t.Count()
	if l == 0 {
		p.SetCount(uint16(l))
	} else if l < 0xFFFF {
		p.SetCount(uint16(l))
		data := common.UnsafeAdd(unsafe.Pointer(p), unsafe.Sizeof(*p))
		ids := unsafe.Slice((*common.Pgid)(data), l)
		t.Copyall(ids)
	} else {
		p.SetCount(0xFFFF)
		data := common.UnsafeAdd(unsafe.Pointer(p), unsafe.Sizeof(*p))
		ids := unsafe.Slice((*common.Pgid)(data), l+1)
		ids[0] = common.Pgid(l)
		t.Copyall(ids[1:])
	}
}
