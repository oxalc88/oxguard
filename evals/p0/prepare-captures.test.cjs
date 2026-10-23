'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const fs=require('node:fs'),os=require('node:os'),path=require('node:path');
const {prepareCaptures}=require('./prepare-captures.cjs');
test('a copied frozen manifest captures the relocated fixture, never its saved absolute source path',t=>{
  const base=fs.mkdtempSync(path.join(os.tmpdir(),'p0-frozen-'));
  t.after(()=>fs.rmSync(base,{recursive:true,force:true}));
  const original=path.join(base,'old','case'),copy=path.join(base,'copy','case');
  fs.mkdirSync(original,{recursive:true});fs.mkdirSync(copy,{recursive:true});
  fs.writeFileSync(path.join(original,'main.ts'),'throw new Error("wrong original");\n');
  fs.writeFileSync(path.join(copy,'main.ts'),'export const value = 1;\n');
  const manifest=path.join(base,'copy','manifest.json');
  fs.writeFileSync(manifest,JSON.stringify({language:'typescript',fixtures:[{id:'typescript.private_local_types',directory:'case',project:original,task:'Review the contract.'}]}));
  const [capture]=prepareCaptures({manifest,output:path.join(base,'captures'),runtime:{root:path.join(base,'runtime')},binary:'unused-until-executed'});
  assert.equal(fs.readFileSync(path.join(capture.root,'main.ts'),'utf8'),'export const value = 1;\n');
  assert.equal(fs.readFileSync(path.join(original,'main.ts'),'utf8'),'throw new Error("wrong original");\n');
});
