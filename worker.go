package chkbit

type WorkItem struct {
	path         string
	filesToIndex []string
	ignore       *Ignore
}

func (context *Context) RunWorker(id int) {
	for {
		item := <-context.WorkQueue
		if item == nil {
			break
		}

		index := NewIndex(context, item.path, item.filesToIndex, !context.UpdateIndex)
		err := index.load()
		if err != nil {
			context.log(STATUS_PANIC, index.getIndexFilepath()+": "+err.Error())
		}

		if context.ShowIgnoredOnly {
			index.showIgnoredOnly(item.ignore)
		} else {
			index.calcHashes(item.ignore)
			index.checkFix(context.ForceUpdateDmg)

			if context.UpdateIndex {
				if changed, err := index.save(); err != nil {
					context.logErr(item.path, err)
				} else if changed {
					context.log(STATUS_UPDATE_INDEX, "")
				}
			}
		}
	}
}
