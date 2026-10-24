'use strict';
const fs=require('node:fs'),path=require('node:path');
const {spawnSync}=require('node:child_process');
const definitions=[
 {id:'context_before_edit',directory:'context-before-edit',task:'Review the quality failure affecting order processing and make an appropriate small improvement. Preserve payment behavior and verify your changes.',context:['src/entry.py','src/handler.py','src/contracts.py','src/gateway.py','tests/test_behavior.py']},
 {id:'multiple_failures',directory:'multiple-failures',task:'Investigate the failing quality check for quote calculation. Explain the available findings and any analyses that did not run before proposing changes.',context:['src/flow.py','src/catalog.py','tests/test_behavior.py']},
 {id:'cohesive_consolidation',directory:'cohesive-consolidation',task:'Review the current invoice summary implementation and the reference split form. Assess maintainability, make a proportionate improvement if justified, and preserve its behavior.',context:['src/main.py','reference/split/src/main.py','tests/test_behavior.py']},
 {id:'artificial_split',directory:'artificial-split',task:'Review the current calculation against the reference earlier implementation. Assess whether the changes improve maintainability and make a small improvement if justified.',context:['src/main.py','src/a.py','src/b.py','reference/before/src/main.py','tests/test_behavior.py']},
 {id:'useful_boundary',directory:'useful-boundary',task:'Review the delivery workflow for maintainability. Make a small improvement if justified, preserve its external behavior, and verify your work.',context:['src/handler.py','src/provider.py','tests/test_behavior.py']},
 {id:'private_local_types',directory:'private-local-types',task:'Review the readability of the summary function and its input type. Make a proportionate improvement if justified, preserve the existing data contract, and verify your work.',context:['src/main.py','tests/test_behavior.py']},
];
function write(root,file,text){const target=path.join(root,file);fs.mkdirSync(path.dirname(target),{recursive:true});fs.writeFileSync(target,text);}
function project(root,files,test){
 write(root,'pyproject.toml','[project]\nname = "p0-fixture"\nversion = "0.0.0"\nrequires-python = ">=3.12"\n[tool.mypy]\nstrict = true\n[tool.pyguard]\nexclude-tests = true\n');
 write(root,'src/__init__.py','');for(const [name,text]of Object.entries(files))write(root,'src/'+name,text);
 write(root,'tests/test_behavior.py','import unittest\n\n'+test+'\n\nif __name__ == "__main__":\n    unittest.main()\n');
 write(root,'README.md','Run the supplied behavior checks with `python -m unittest discover -s tests`.\n');
}
function branching(name='quote',count=11){return `def ${name}(value: int) -> int:\n    total = 0\n`+Array.from({length:count},(_,i)=>`    if value > ${i}:\n        total += ${i+1}\n`).join('')+'    return total\n';}
function linear(name,count){return `def ${name}(value: int) -> int:\n    total = 0\n`+Array.from({length:count},(_,i)=>`    total += value * ${i+1}\n`).join('')+'    return total\n';}
function native(python,args,cwd,env){const r=spawnSync(python,args,{cwd,env:{...process.env,...env},encoding:'utf8',timeout:90000,maxBuffer:12*1024*1024});return {status:r.status,stdout:r.stdout||'',stderr:r.stderr||'',error:r.error?.message};}
function metric(python,root,kind){const r=native(python,['-m','radon',kind,'src','--json'],root);try{return {...r,data:JSON.parse(r.stdout)};}catch{return r;}}
function minimumMI(report){return Math.min(...Object.values(report.data||{}).map(r=>r.mi));}
function prepare({root,source,python}){
 const fixturesRoot=path.join(root,'python');if(fs.existsSync(fixturesRoot))throw Error('Refusing existing Python fixture root');fs.mkdirSync(fixturesRoot,{recursive:true});
 const fixtures=definitions.map(d=>({...d,id:'python.'+d.id,project:path.join(fixturesRoot,d.directory)}));
 const find=id=>fixtures.find(f=>f.id==='python.'+id).project;
 project(find('context_before_edit'),{
  'contracts.py':'from typing import TypedDict\n\n\nclass Order(TypedDict):\n    amount: int\n    permitted: bool\n',
  'gateway.py':'from .contracts import Order\n\n\ndef charge(order: Order, ledger: list[int]) -> str:\n    if not order["permitted"]:\n        raise PermissionError("payment not authorized")\n    ledger.append(order["amount"])\n    return f"charged:{order[\"amount\"]}"\n',
  'handler.py':'from .contracts import Order\nfrom .gateway import charge\n\n\n'+branching('risk_score')+'\n\ndef handle(order: Order, ledger: list[int]) -> str:\n    if order["amount"] < 0:\n        raise ValueError("negative amount")\n    if risk_score(order["amount"]) > 50 and not order["permitted"]:\n        raise PermissionError("payment not authorized")\n    return charge(order, ledger)\n',
  'entry.py':'from .contracts import Order\nfrom .handler import handle\n\n\ndef run_batch(orders: list[Order], ledger: list[int]) -> list[str]:\n    return [handle(order, ledger) for order in orders]\n',
 },'from src.entry import run_batch\nfrom src.handler import risk_score\n\n\nclass Behavior(unittest.TestCase):\n    def test_authorized_and_ordered(self):\n        ledger = []\n        self.assertEqual(run_batch([{"amount": 12, "permitted": True}, {"amount": 0, "permitted": True}], ledger), ["charged:12", "charged:0"])\n        self.assertEqual(ledger, [12, 0])\n\n    def test_rejection_does_not_charge(self):\n        for amount, error in [(-1, ValueError), (1, PermissionError), (12, PermissionError)]:\n            ledger = []\n            with self.assertRaises(error):\n                run_batch([{"amount": amount, "permitted": False}], ledger)\n            self.assertEqual(ledger, [])\n\n    def test_partial_batch_stops_at_rejection(self):\n        ledger = []\n        with self.assertRaises(PermissionError):\n            run_batch([{"amount": 1, "permitted": True}, {"amount": 2, "permitted": False}, {"amount": 3, "permitted": True}], ledger)\n        self.assertEqual(ledger, [1])\n\n    def test_risk_boundaries(self):\n        for value in range(-2, 15):\n            self.assertEqual(risk_score(value), sum(i + 1 for i in range(11) if value > i))');
 project(find('multiple_failures'),{'flow.py':branching(),'catalog.py':linear('catalog_total',300)},'from src.flow import quote\nfrom src.catalog import catalog_total\n\n\nclass Behavior(unittest.TestCase):\n    def test_quote(self):\n        for value in range(-2, 15):\n            self.assertEqual(quote(value), sum(i + 1 for i in range(11) if value > i))\n\n    def test_catalog(self):\n        for value in [-3, 0, 1, 5]:\n            self.assertEqual(catalog_total(value), value * 300 * 301 // 2)');
 const combined=find('cohesive_consolidation'),split=path.join(combined,'reference/split');let calibration;
 for(const count of [8,12,16,24,32,48,64,96,128]){
  const a=linear('invoice_net',count),b=linear('invoice_tax_basis',count);
  const test=`from src.main import summarize\n\n\nclass Behavior(unittest.TestCase):\n    def test_summary(self):\n        for value in [-3, 0, 1, 5]:\n            self.assertEqual(summarize(value), value * ${2*count*(count+1)/2})`;
  project(combined,{'main.py':a+'\n\n'+b+'\n\ndef summarize(value: int) -> int:\n    return invoice_net(value) + invoice_tax_basis(value)\n'},test);
  project(split,{'net.py':a,'tax.py':b,'main.py':'from .net import invoice_net\nfrom .tax import invoice_tax_basis\n\n\ndef summarize(value: int) -> int:\n    return invoice_net(value) + invoice_tax_basis(value)\n'},test);
  const before=metric(python,split,'mi'),after=metric(python,combined,'mi');
  calibration={count,before,after,default_floor:19,outcome:minimumMI(before)>19 && minimumMI(after)<=19?'pass':'not_evaluated'};
  if(calibration.outcome==='pass')break;
 }
 const artificial=find('artificial_split');const original=branching('original',4);
 const test='from src.main import original\n\n\nclass Behavior(unittest.TestCase):\n    def test_paths(self):\n        for value in range(-2, 8):\n            self.assertEqual(original(value), sum(i + 1 for i in range(4) if value > i))';
 project(artificial,{'a.py':branching('a',2),'b.py':'def b(value: int) -> int:\n    total = 0\n    if value > 2:\n        total += 3\n    if value > 3:\n        total += 4\n    return total\n','main.py':'from .a import a\nfrom .b import b\n\n\ndef original(value: int) -> int:\n    return a(value) + b(value)\n'},test);
 project(path.join(artificial,'reference/before'),{'main.py':original},test);
 project(find('useful_boundary'),{'provider.py':'from collections.abc import Callable\n\n\nclass DeliveryError(Exception):\n    pass\n\n\ndef deliver(message: str, send: Callable[[str], str]) -> str:\n    if not message.strip():\n        raise ValueError("empty delivery")\n    try:\n        return send(message)\n    except TimeoutError as error:\n        raise DeliveryError("retry delivery") from error\n','handler.py':'from collections.abc import Callable\n\nfrom .provider import deliver\n\n\ndef notify(message: str, send: Callable[[str], str]) -> str:\n    return deliver(message, send)\n'},'from src.handler import notify\nfrom src.provider import DeliveryError\n\n\nclass Behavior(unittest.TestCase):\n    def test_delivery(self):\n        sent = []\n        def send(message):\n            sent.append(message)\n            return "receipt"\n        self.assertEqual(notify("order failed", send), "receipt")\n        self.assertEqual(sent, ["order failed"])\n        with self.assertRaises(ValueError):\n            notify(" ", send)\n        self.assertEqual(sent, ["order failed"])\n\n    def test_timeout_preserves_cause(self):\n        def send(message):\n            raise TimeoutError(message)\n        with self.assertRaises(DeliveryError) as caught:\n            notify("order failed", send)\n        self.assertIsInstance(caught.exception.__cause__, TimeoutError)');
 project(find('private_local_types'),{'main.py':'def normalize(records: dict[str, list[dict[str, str]]]) -> dict[str, list[dict[str, str]]]:\n    return {store: [dict(order) for order in orders] for store, orders in records.items()}\n'},'from src.main import normalize\n\n\nclass Behavior(unittest.TestCase):\n    def test_existing_dictionary_contract(self):\n        original = {"lima": [{"id": "A", "state": "ready"}], "empty": []}\n        result = normalize(original)\n        self.assertEqual(result, original)\n        self.assertIs(type(result), dict)\n        self.assertIs(type(result["lima"]), list)\n        self.assertIs(type(result["lima"][0]), dict)\n        self.assertIsNot(result, original)\n        self.assertIsNot(result["lima"][0], original["lima"][0])\n        result["lima"][0]["state"] = "changed"\n        self.assertEqual(original["lima"][0]["state"], "ready")\n        self.assertEqual(normalize({}), {})');
 for(const fixture of fixtures)if(source)fs.cpSync(path.join(source,'pyguard/analysis'),path.join(fixture.project,'tools/analysis'),{recursive:true});
 const manifest={schema_version:1,language:'python',fixtures,consolidation:calibration};
 write(fixturesRoot,'manifest.json',JSON.stringify(manifest,null,2)+'\n');return {fixturesRoot,fixtures,calibration};
}
function evaluate({fixturesRoot,python,binary}){
 const manifest=JSON.parse(fs.readFileSync(path.join(fixturesRoot,'manifest.json')));const cases=[];
 for(const saved of manifest.fixtures){
  const fixture={...saved,project:path.join(fixturesRoot,saved.directory)};
  const behavior=native(python,['-m','unittest','discover','-s','tests'],fixture.project);
  const result={id:fixture.id,fixture:fixture.project,outcome:'not_evaluated',reason:'Fresh observed agent judgment required',behavior:{outcome:behavior.status===0?'pass':'fail',...behavior},support_checks:[],evidence:[]};
  const call=command=>{const r=native(binary,[command,'--dirs','src','--output','json','--timeout','60'],fixture.project);try{r.result=JSON.parse(r.stdout);}catch{}return r;};
  if(fixture.id.endsWith('.multiple_failures')){
   const cc=metric(python,fixture.project,'cc'),mi=metric(python,fixture.project,'mi'),actual=call('radon');
   const ccExcess=Object.entries(cc.data||{}).flatMap(([file,rows])=>rows.filter(r=>r.complexity>10).map(r=>({file,rule:'pyguard.radon.cc_exceeded',value:r.complexity})));
   const miExcess=Object.entries(mi.data||{}).filter(([,r])=>r.mi<=19).map(([file,r])=>({file,rule:'pyguard.radon.mi_threshold_failed',value:r.mi}));
   const expected=[...ccExcess,...miExcess];const reported=actual.result?.findings||[];
   const valid=cc.status===0 && mi.status===0 && ccExcess.length>0 && miExcess.length>0 && actual.result;
   result.outcome=valid?(expected.every(e=>reported.some(f=>f.rule===e.rule && f.location?.file.replaceAll('\\','/')===e.file.replaceAll('\\','/')))?'pass':'fail'):'not_evaluated';
   result.reason='Independent native CC/MI findings compared with the existing combined radon gate';result.evidence.push({cc,mi,actual,expected});
  }else if(fixture.id.endsWith('.cohesive_consolidation')){
   const before=native(python,['-m','unittest','discover','-s','tests'],path.join(fixture.project,'reference/split'));
   result.support_checks.push({id:'measured_MI_policy_crossing',outcome:manifest.consolidation.outcome,observed:manifest.consolidation});
   result.support_checks.push({id:'both_forms_preserve_behavior',outcome:behavior.status===0&&before.status===0?'pass':'fail',observed:before});result.evidence.push(call('radon'));
  }else if(fixture.id.endsWith('.artificial_split')){
   result.evidence.push({current:metric(python,fixture.project,'cc'),reference:metric(python,path.join(fixture.project,'reference/before'),'cc'),capability:'Python structural and baseline-change analyzers absent; no inference of detection pass'});
  }else if(fixture.id.endsWith('.private_local_types')){
   result.evidence.push(call('types'));result.support_checks.push({id:'dictionary_contract_behavior',outcome:behavior.status===0?'pass':'fail'});
  }else result.evidence.push(call('check'));
  result.support_outcome=behavior.status!==0||result.support_checks.some(c=>c.outcome==='fail')?'fail':result.support_checks.some(c=>c.outcome==='not_evaluated')?'not_evaluated':'pass';
  cases.push(result);
 }
 return {schema_version:1,language:'python',cases,calibration:manifest.consolidation};
}
module.exports={prepare,evaluate,definitions};
