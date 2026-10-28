import { describe, expect, it } from 'vitest'
import { relsPathFor, slideImageTargets, sortSlideNames } from '~/utils/pptx'

describe('pptx', () => {
  it('sorts slides numerically', () => {
    expect(sortSlideNames(['ppt/slides/slide10.xml', 'ppt/slides/slide2.xml', 'ppt/slides/slide1.xml', 'ppt/media/image1.png']))
      .toEqual(['ppt/slides/slide1.xml', 'ppt/slides/slide2.xml', 'ppt/slides/slide10.xml'])
  })
  it('maps a slide to only its own images', () => {
    const rels = `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
      <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>
      <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/image3.png"/>
      <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/image3.png"/>
      <Relationship Id="rId4" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="https://x/y.png" TargetMode="External"/>
      <Relationship Id="rId5" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="/ppt/media/image9.jpeg"/>
    </Relationships>`
    expect(slideImageTargets(rels)).toEqual(['ppt/media/image3.png', 'ppt/media/image9.jpeg'])
  })
  it('builds the rels path', () => {
    expect(relsPathFor('ppt/slides/slide7.xml')).toBe('ppt/slides/_rels/slide7.xml.rels')
  })
})
