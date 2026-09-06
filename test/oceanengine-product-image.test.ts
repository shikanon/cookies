import assert from 'node:assert/strict'
import test from 'node:test'
import { isOceanEngineImageSourceIdentity, oceanEngineImageSourceIdentity } from '../src/lib/oceanengine-product-image.ts'

test('product image source identity removes transient signed URL data', () => {
  const webURI = 'tos-cn-i-sd07hgqsbj/26d59f497e2540a3a377554cd94e4706'
  const signedURL = `https://p0-adplatform-private.oceanengine.com/${webURI}~tplv-iq460dd072-origin.image?x-orig-expires=1788024626&x-orig-sign=changed`
  assert.equal(oceanEngineImageSourceIdentity(webURI), webURI)
  assert.equal(oceanEngineImageSourceIdentity(signedURL), webURI)
  assert.equal(oceanEngineImageSourceIdentity('/api/connector/v1/projects/project-1/platform-objects/image-1/preview'), '')
  assert.equal(isOceanEngineImageSourceIdentity(webURI), true)
  assert.equal(isOceanEngineImageSourceIdentity('api/connector/v1/projects/project-1/platform-objects/image-1/preview'), false)
  assert.equal(oceanEngineImageSourceIdentity(''), '')
})
