import test from 'node:test';
import assert from 'node:assert/strict';
import { FeedbackController } from '../dist/feedback.js';

test('feedback defaults remain opt-in; constructor colors accept short and full hex',()=>{
  const defaults={cursor:false,edgeGlow:false,edgeGlowColor:'#4f8fe8'};
  assert.deepEqual(new FeedbackController().configuration,defaults);
  assert.deepEqual(new FeedbackController({cursor:undefined,edgeGlow:undefined,edgeGlowColor:undefined}).configuration,defaults);
  assert.deepEqual(new FeedbackController({cursor:true,edgeGlowColor:'#A3c'}).configuration,
    {cursor:true,edgeGlow:false,edgeGlowColor:'#aa33cc'});
  assert.equal(new FeedbackController({edgeGlowColor:'#12AbCd'}).configuration.edgeGlowColor,'#12abcd');
});

test('partial feedback updates keep the chosen color and independent toggles',async()=>{
  const feedback=new FeedbackController({cursor:true,edgeGlow:true,edgeGlowColor:'#a3c'});
  await feedback.configure({edgeGlow:false});
  assert.deepEqual(feedback.configuration,{cursor:true,edgeGlow:false,edgeGlowColor:'#aa33cc'});
  await feedback.configure({edgeGlowColor:'#22C55E'});
  assert.equal(feedback.configuration.edgeGlow,false);
  await feedback.configure({edgeGlow:true,cursor:undefined,edgeGlowColor:undefined});
  assert.deepEqual(feedback.configuration,{cursor:true,edgeGlow:true,edgeGlowColor:'#22c55e'});
  const copy=feedback.configuration; copy.edgeGlowColor='#ffffff';
  assert.equal(feedback.configuration.edgeGlowColor,'#22c55e');
});

test('invalid feedback colors and flags fail atomically before changing configuration',async()=>{
  const feedback=new FeedbackController({cursor:true,edgeGlow:true});
  const before=feedback.configuration;
  for (const color of ['',null,42,{},'blue','#12','#1234','#12345678','#gggggg','#fff;display:none','var(--page-color)']) {
    assert.throws(()=>new FeedbackController({edgeGlowColor:color}),/hex color/);
    await assert.rejects(feedback.configure({edgeGlow:false,edgeGlowColor:color}),/hex color/);
    assert.deepEqual(feedback.configuration,before);
  }
  await assert.rejects(feedback.configure({cursor:'yes',edgeGlowColor:'#abc'}),/boolean/);
  assert.deepEqual(feedback.configuration,before);
});
