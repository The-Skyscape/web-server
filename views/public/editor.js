/**
 * ThoughtEditor - A Notion-like block editor
 * Uses contenteditable for inline editing with Hyperscript for interactions
 */

class ThoughtEditor {
  constructor(container, thoughtId, host = '') {
    this.container = container
    this.thoughtId = thoughtId
    this.host = host
    this.blocksContainer = container.querySelector('#editor-blocks')
    this.commandPalette = container.querySelector('#command-palette')
    this.autoSaveTimers = {}
    this.currentBlock = null

    this.init()
  }

  init() {
    // Set up event delegation for blocks
    this.blocksContainer.addEventListener('keydown', (e) => this.handleKeydown(e))
    this.blocksContainer.addEventListener('input', (e) => this.handleInput(e))
    this.blocksContainer.addEventListener('paste', (e) => this.handlePaste(e))
    this.blocksContainer.addEventListener('focus', (e) => this.handleFocus(e), true)

    // Command palette events
    if (this.commandPalette) {
      this.commandPalette.addEventListener('click', (e) => this.handlePaletteClick(e))
    }

    // Close palette on outside click
    document.addEventListener('click', (e) => {
      if (this.commandPalette && !this.commandPalette.contains(e.target)) {
        this.hidePalette()
      }
    })

    // Initialize empty editor with a paragraph block
    if (this.blocksContainer.children.length === 0) {
      this.createBlock('paragraph', 1)
    }
  }

  // API Methods

  async createBlock(type, position, content = '', fileId = '', metadata = '') {
    const response = await fetch(`${this.host}/thought/${this.thoughtId}/blocks`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        type,
        position,
        content,
        file_id: fileId,
        metadata,
      }),
    })

    if (!response.ok) {
      console.error('Failed to create block')
      return null
    }

    const block = await response.json()
    this.renderBlock(block, position)
    return block
  }

  async updateBlock(blockId, updates) {
    const response = await fetch(`${this.host}/thought/${this.thoughtId}/block/${blockId}`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(updates),
    })

    if (!response.ok) {
      console.error('Failed to update block')
      return false
    }

    return true
  }

  async deleteBlock(blockId) {
    const response = await fetch(`${this.host}/thought/${this.thoughtId}/block/${blockId}`, {
      method: 'DELETE',
    })

    if (!response.ok) {
      console.error('Failed to delete block')
      return false
    }

    const blockEl = this.blocksContainer.querySelector(`[data-block-id="${blockId}"]`)
    if (blockEl) {
      const prevBlock = blockEl.previousElementSibling
      blockEl.remove()

      // Focus previous block
      if (prevBlock) {
        const content = prevBlock.querySelector('.block-content')
        if (content) {
          content.focus()
          this.moveCursorToEnd(content)
        }
      }
    }

    return true
  }

  async uploadFile(file) {
    const formData = new FormData()
    formData.append('file', file)

    const response = await fetch(`${this.host}/files`, {
      method: 'POST',
      headers: {
        'Accept': 'application/json',
      },
      body: formData,
    })

    if (!response.ok) {
      console.error('Failed to upload file')
      return null
    }

    return await response.json()
  }

  // Auto-save

  scheduleAutoSave(blockId, content) {
    if (this.autoSaveTimers[blockId]) {
      clearTimeout(this.autoSaveTimers[blockId])
    }

    this.autoSaveTimers[blockId] = setTimeout(() => {
      this.updateBlock(blockId, { content })
      delete this.autoSaveTimers[blockId]
      this.showSaveIndicator()
    }, 2000)
  }

  showSaveIndicator() {
    // Brief visual feedback that save occurred
    const indicator = document.getElementById('save-indicator')
    if (indicator) {
      indicator.classList.remove('opacity-0')
      indicator.classList.add('opacity-100')
      setTimeout(() => {
        indicator.classList.remove('opacity-100')
        indicator.classList.add('opacity-0')
      }, 1000)
    }
  }

  // Event Handlers

  handleKeydown(e) {
    const blockEl = e.target.closest('.editor-block')
    if (!blockEl) return

    const blockId = blockEl.dataset.blockId
    const content = e.target

    // Enter - create new block
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      const position = parseInt(blockEl.dataset.position) + 1
      this.createBlockAfter(blockEl)
      return
    }

    // Backspace on empty - delete block
    if (e.key === 'Backspace' && content.textContent.trim() === '') {
      const blocks = this.blocksContainer.querySelectorAll('.editor-block')
      if (blocks.length > 1) {
        e.preventDefault()
        this.deleteBlock(blockId)
      }
      return
    }

    // / at start - show command palette
    if (e.key === '/' && this.getCursorPosition(content) === 0) {
      e.preventDefault()
      this.showPalette(blockEl)
      return
    }

    // Arrow up at start - focus previous block
    if (e.key === 'ArrowUp' && this.getCursorPosition(content) === 0) {
      const prevBlock = blockEl.previousElementSibling
      if (prevBlock) {
        e.preventDefault()
        const prevContent = prevBlock.querySelector('.block-content')
        if (prevContent) {
          prevContent.focus()
          this.moveCursorToEnd(prevContent)
        }
      }
      return
    }

    // Arrow down at end - focus next block
    if (e.key === 'ArrowDown' && this.getCursorPosition(content) === content.textContent.length) {
      const nextBlock = blockEl.nextElementSibling
      if (nextBlock) {
        e.preventDefault()
        const nextContent = nextBlock.querySelector('.block-content')
        if (nextContent) {
          nextContent.focus()
          this.moveCursorToStart(nextContent)
        }
      }
      return
    }

    // Escape - hide palette
    if (e.key === 'Escape') {
      this.hidePalette()
      return
    }
  }

  handleInput(e) {
    const blockEl = e.target.closest('.editor-block')
    if (!blockEl) return

    const blockId = blockEl.dataset.blockId
    const content = e.target.innerHTML

    this.scheduleAutoSave(blockId, content)
  }

  handleFocus(e) {
    const blockEl = e.target.closest('.editor-block')
    if (blockEl) {
      this.currentBlock = blockEl
    }
  }

  handlePaste(e) {
    const blockEl = e.target.closest('.editor-block')
    if (!blockEl) return

    // Check for image paste
    const items = e.clipboardData?.items
    if (items) {
      for (const item of items) {
        if (item.type.startsWith('image/')) {
          e.preventDefault()
          const file = item.getAsFile()
          this.insertImageFromFile(file, blockEl)
          return
        }
      }
    }

    // For text, strip HTML and paste plain text
    e.preventDefault()
    const text = e.clipboardData?.getData('text/plain') || ''
    document.execCommand('insertText', false, text)
  }

  handlePaletteClick(e) {
    const btn = e.target.closest('[data-block-type]')
    if (!btn) return

    const type = btn.dataset.blockType
    this.hidePalette()

    if (!this.currentBlock) return

    const position = parseInt(this.currentBlock.dataset.position)

    if (type === 'image') {
      this.showFileUpload('image', position)
    } else if (type === 'file') {
      this.showFileUpload('file', position)
    } else {
      // Convert current block or insert new one
      const currentContent = this.currentBlock.querySelector('.block-content')
      if (currentContent && currentContent.textContent.trim() === '') {
        // Convert current block type
        this.convertBlockType(this.currentBlock.dataset.blockId, type)
      } else {
        // Insert new block after current
        this.createBlock(type, position + 1)
      }
    }
  }

  // Block Operations

  async createBlockAfter(blockEl) {
    const position = parseInt(blockEl.dataset.position) + 1
    const block = await this.createBlock('paragraph', position)

    if (block) {
      // Focus the new block
      setTimeout(() => {
        const newBlockEl = this.blocksContainer.querySelector(`[data-block-id="${block.ID}"]`)
        if (newBlockEl) {
          const content = newBlockEl.querySelector('.block-content')
          if (content) content.focus()
        }
      }, 50)
    }
  }

  async convertBlockType(blockId, newType) {
    const blockEl = this.blocksContainer.querySelector(`[data-block-id="${blockId}"]`)
    if (!blockEl) return

    let metadata = ''
    if (newType === 'heading') {
      metadata = JSON.stringify({ level: 2 })
    }

    await this.updateBlock(blockId, { type: newType, metadata })

    // Re-render the block
    blockEl.dataset.type = newType
    this.updateBlockAppearance(blockEl, newType)
  }

  updateBlockAppearance(blockEl, type) {
    const content = blockEl.querySelector('.block-content')
    if (!content) return

    // Remove existing type classes
    content.classList.remove('text-3xl', 'text-2xl', 'text-xl', 'font-bold',
      'border-l-4', 'border-white/20', 'pl-4', 'italic',
      'bg-base-200', 'p-4', 'rounded', 'font-mono', 'text-sm')

    // Add type-specific classes
    switch (type) {
      case 'heading':
        content.classList.add('text-2xl', 'font-bold')
        break
      case 'quote':
        content.classList.add('border-l-4', 'border-white/20', 'pl-4', 'italic')
        break
      case 'code':
        content.classList.add('bg-base-200', 'p-4', 'rounded', 'font-mono', 'text-sm')
        break
    }
  }

  // File Upload

  showFileUpload(type, position) {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = type === 'image' ? 'image/*' : '*/*'

    input.onchange = async (e) => {
      const file = e.target.files[0]
      if (!file) return

      const uploaded = await this.uploadFile(file)
      if (uploaded) {
        await this.createBlock(type, position + 1, '', uploaded.id)
      }
    }

    input.click()
  }

  async insertImageFromFile(file, afterBlock) {
    const position = parseInt(afterBlock.dataset.position)
    const uploaded = await this.uploadFile(file)

    if (uploaded) {
      await this.createBlock('image', position + 1, '', uploaded.id)
    }
  }

  // Command Palette

  showPalette(blockEl) {
    if (!this.commandPalette) return

    this.currentBlock = blockEl
    const rect = blockEl.getBoundingClientRect()

    this.commandPalette.style.top = `${rect.bottom + window.scrollY + 8}px`
    this.commandPalette.style.left = `${rect.left + window.scrollX}px`
    this.commandPalette.classList.remove('hidden')
  }

  hidePalette() {
    if (this.commandPalette) {
      this.commandPalette.classList.add('hidden')
    }
  }

  // Block Rendering

  renderBlock(block, position) {
    const html = this.createBlockHTML(block)
    const temp = document.createElement('div')
    temp.innerHTML = html

    const newBlockEl = temp.firstElementChild

    // Find insertion point
    const blocks = Array.from(this.blocksContainer.querySelectorAll('.editor-block'))
    const insertBefore = blocks.find(b => parseInt(b.dataset.position) >= position)

    if (insertBefore) {
      this.blocksContainer.insertBefore(newBlockEl, insertBefore)
      // Update positions of subsequent blocks
      blocks.forEach(b => {
        const pos = parseInt(b.dataset.position)
        if (pos >= position) {
          b.dataset.position = pos + 1
        }
      })
    } else {
      this.blocksContainer.appendChild(newBlockEl)
    }

    // Focus the new block
    const content = newBlockEl.querySelector('.block-content')
    if (content) {
      content.focus()
    }
  }

  createBlockHTML(block) {
    const typeClasses = this.getTypeClasses(block.Type)

    if (block.Type === 'image' && block.FileID) {
      return `
        <div class="editor-block group flex gap-2 items-start"
             data-block-id="${block.ID}"
             data-type="${block.Type}"
             data-position="${block.Position}">
          <div class="block-handle cursor-grab opacity-0 group-hover:opacity-60 pt-2 text-white/40">⋮⋮</div>
          <div class="flex-1">
            <img src="${this.host}/file/${block.FileID}" class="max-w-full rounded-lg" alt="${block.Content || ''}">
            <input type="text" value="${block.Content || ''}" placeholder="Add a caption..."
              class="input input-sm input-ghost w-full mt-2 text-white/60"
              onchange="window.thoughtEditor.updateBlock('${block.ID}', { content: this.value })">
          </div>
        </div>
      `
    }

    if (block.Type === 'file' && block.FileID) {
      return `
        <div class="editor-block group flex gap-2 items-center"
             data-block-id="${block.ID}"
             data-type="${block.Type}"
             data-position="${block.Position}">
          <div class="block-handle cursor-grab opacity-0 group-hover:opacity-60 text-white/40">⋮⋮</div>
          <a href="${this.host}/file/${block.FileID}" target="_blank"
             class="flex items-center gap-2 p-3 bg-base-200 rounded-lg hover:bg-base-300 transition-colors">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z"/>
            </svg>
            <span>${block.Content || 'Attached file'}</span>
          </a>
        </div>
      `
    }

    return `
      <div class="editor-block group flex gap-2 items-start"
           data-block-id="${block.ID}"
           data-type="${block.Type}"
           data-position="${block.Position}">
        <div class="block-handle cursor-grab opacity-0 group-hover:opacity-60 pt-1 text-white/40">⋮⋮</div>
        <div class="block-content flex-1 outline-none min-h-[1.5em] ${typeClasses}"
             contenteditable="true"
             data-placeholder="Type '/' for commands...">${block.Content || ''}</div>
      </div>
    `
  }

  getTypeClasses(type) {
    switch (type) {
      case 'heading':
        return 'text-2xl font-bold'
      case 'quote':
        return 'border-l-4 border-white/20 pl-4 italic text-white/80'
      case 'code':
        return 'bg-base-200 p-4 rounded font-mono text-sm whitespace-pre-wrap'
      default:
        return ''
    }
  }

  // Cursor Utilities

  getCursorPosition(element) {
    const selection = window.getSelection()
    if (!selection.rangeCount) return 0

    const range = selection.getRangeAt(0)
    const preRange = range.cloneRange()
    preRange.selectNodeContents(element)
    preRange.setEnd(range.startContainer, range.startOffset)

    return preRange.toString().length
  }

  moveCursorToEnd(element) {
    const range = document.createRange()
    const selection = window.getSelection()
    range.selectNodeContents(element)
    range.collapse(false)
    selection.removeAllRanges()
    selection.addRange(range)
  }

  moveCursorToStart(element) {
    const range = document.createRange()
    const selection = window.getSelection()
    range.selectNodeContents(element)
    range.collapse(true)
    selection.removeAllRanges()
    selection.addRange(range)
  }
}

// Initialize editor when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
  const editorContainer = document.getElementById('thought-editor')
  if (editorContainer) {
    const thoughtId = editorContainer.dataset.thoughtId
    const host = editorContainer.dataset.host || ''
    window.thoughtEditor = new ThoughtEditor(editorContainer, thoughtId, host)
  }
})

// Also handle HTMX page swaps
document.addEventListener('htmx:afterSettle', () => {
  const editorContainer = document.getElementById('thought-editor')
  if (editorContainer && !window.thoughtEditor) {
    const thoughtId = editorContainer.dataset.thoughtId
    const host = editorContainer.dataset.host || ''
    window.thoughtEditor = new ThoughtEditor(editorContainer, thoughtId, host)
  }
})
