package chkbit

type WorkItem struct {
	path         string
	filesToIndex []string
	dirList      []string
	ignore       *Ignore
}

func (context *Context) runWorker(id int) {
	for {
		item := <-context.WorkQueue
		if item == nil {
			break
		}

		index := newIndex(context, item.path, item.filesToIndex, item.dirList, !context.UpdateIndex)
		err := index.load()
		if err != nil {
			context.logErr(index.getIndexFilepath(), err)
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
