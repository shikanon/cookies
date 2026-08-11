import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const componentPath = new URL('../src/components/SpecializedPages.tsx', import.meta.url)
const editorPath = new URL('../src/features/video-editing/VideoEditingWorkspace.tsx', import.meta.url)
const canvasEditorPath = new URL('../src/features/video-editing/VideoEditingCanvasWorkspace.tsx', import.meta.url)
const stylesPath = new URL('../src/styles.css', import.meta.url)

test('video editor keeps the full inspector reachable above the fixed status bar', async () => {
  const styles = await readFile(stylesPath, 'utf8')

  assert.match(styles, /\.video-editing-workspace \.editing-shell \{[^}]*min-height:\s*560px/s)
  assert.match(styles, /\.video-editing-workspace \.editing-inspector \{[^}]*overflow-y:\s*auto/s)
  assert.doesNotMatch(styles, /\.video-editing-workspace \.editing-inspector \{[^}]*overflow:\s*hidden/s)
})

test('vertical video preview is constrained to the canvas instead of overflowing its grid row', async () => {
  const styles = await readFile(stylesPath, 'utf8')

  assert.match(styles, /\.real-preview video \{[^}]*position:\s*absolute[^}]*inset:\s*0[^}]*object-fit:\s*contain/s)
})

test('video creation routes material editing to the interactive timeline workspace', async () => {
  const [component, editor, canvasEditor] = await Promise.all([
    readFile(componentPath, 'utf8'),
    readFile(editorPath, 'utf8'),
    readFile(canvasEditorPath, 'utf8'),
  ])

  assert.match(component, /<VideoEditingWorkspaceV2/)
  assert.match(editor, /VideoEditingCanvasWorkspace/)
  assert.match(canvasEditor, /加入所选轨道/)
  assert.match(canvasEditor, /低清预览/)
  assert.match(canvasEditor, />分割</)
  assert.match(canvasEditor, />删除</)
  assert.match(canvasEditor, />撤销</)
  assert.match(canvasEditor, />重做</)
  assert.match(canvasEditor, /载入服务端版本/)
  assert.match(canvasEditor, /另存为新 EditTask/)
  assert.match(canvasEditor, /cause\.status === 409/)
  assert.doesNotMatch(canvasEditor, /渲染引擎已支持，MVP 暂无可视化编辑/)
})
