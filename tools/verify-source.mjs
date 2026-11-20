import fs from 'node:fs/promises';
import path from 'node:path';
import crypto from 'node:crypto';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import {fileURLToPath} from 'node:url';
const exec=promisify(execFile);
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const modulePath='github.com/MalenkiySolovey/solovey-backend';
const productPrefix='github.com/MalenkiySolovey/solovey-ui/backend';
// M3.3's accepted Backend export is immutable. Maintained files are compared
// to their original Product sources at this revision, not frozen afterwards.
const exportedRevision='73338525c2eac5de146004d515547ce813bc36ac';
const digest=b=>crypto.createHash('sha256').update(b).digest('hex');
const provenance=JSON.parse(await fs.readFile(path.join(root,'SOURCE_PROVENANCE.json'),'utf8'));
if(provenance.sourceRepository!=='https://github.com/MalenkiySolovey/solovey-ui'||!/^[a-f0-9]{40}$/.test(provenance.sourceCommit)||!/^[a-f0-9]{40}$/.test(provenance.sourceTree)||!provenance.files.length)throw Error('Invalid original source identity');
const selected=new Set();
// New Backend-owned source is reviewed and pinned by the Backend Git tree,
// not retroactively attributed to the historical Product export.
const native=new Set(['storage/sqlite/transaction.go','storage/sqlite/transaction_test.go','users/entitlement/entitlement.go','users/entitlement/entitlement_test.go']);
for(const file of provenance.files){
 if(!file.output||file.output.includes('\\')||path.posix.normalize(file.output)!==file.output||file.output.startsWith('../')||path.isAbsolute(file.output)||selected.has(file.output))throw Error('Invalid source projection path');
 if(file.source!=='LICENSE'&&file.source!=='backend/'+file.output)throw Error('Source owner path differs');
 if(!/^[a-f0-9]{40}$/.test(file.sourceBlob)||!/^[a-f0-9]{64}$/.test(file.sourceSHA256)||!/^[a-f0-9]{64}$/.test(file.outputSHA256))throw Error('Invalid source hash');
 let content=await fs.readFile(path.join(root,file.output));
 if(digest(content)!==file.outputSHA256){
  const originalExport=await exec('git',['cat-file','blob',`${exportedRevision}:${file.output}`],{cwd:root,encoding:'buffer',maxBuffer:16<<20,windowsHide:true});
  content=originalExport.stdout;
 }
 if(digest(content)!==file.outputSHA256)throw Error('Exported source differs: '+file.output);
 const original=file.output.endsWith('.go')?Buffer.from(content.toString('utf8').replaceAll(modulePath,productPrefix)):content;
 if(digest(original)!==file.sourceSHA256)throw Error('Source adaptation exceeds the canonical import rewrite: '+file.output);
 selected.add(file.output);
}
async function walk(directory){let files=[];for(const item of await fs.readdir(directory,{withFileTypes:true})){if(item.name==='.git')continue;const full=path.join(directory,item.name);if(item.isSymbolicLink())throw Error('Unexpected source symlink');if(item.isDirectory())files.push(...await walk(full));else files.push(full);}return files;}
for(const full of await walk(root)){
 const relative=path.relative(root,full).split(path.sep).join('/');
 if(!relative.endsWith('.go'))continue;
 if(!selected.has(relative)&&!native.delete(relative))throw Error('Go source has no provenance: '+relative);
 const content=await fs.readFile(full,'utf8');
 if(/github\.com\/MalenkiySolovey\/solovey-(?:ui|node)(?:\/|["\s])/.test(content))throw Error('Forbidden private owner import: '+relative);
 if(/-----BEGIN (?:RSA |EC |OPENSSH |ENCRYPTED )?PRIVATE KEY-----|gh[pousr]_[A-Za-z0-9_]{30,}|AKIA[0-9A-Z]{16}/.test(content))throw Error('Possible private credential in exported source: '+relative);
}
if(native.size)throw Error('Missing reviewed Backend source: '+[...native].join(', '));
function parseStream(text){let values=[],start=-1,depth=0,quoted=false,escaped=false;for(let i=0;i<text.length;i++){const c=text[i];if(start<0){if(/\s/.test(c))continue;if(c!=='{')throw Error('Unexpected JSON stream');start=i;}if(quoted){if(escaped)escaped=false;else if(c==='\\')escaped=true;else if(c==='"')quoted=false;}else if(c==='"')quoted=true;else if(c==='{')depth++;else if(c==='}'){depth--;if(!depth){values.push(JSON.parse(text.slice(start,i+1)));start=-1;}}}if(start!==-1)throw Error('Truncated JSON stream');return values;}
const options={cwd:root,env:{...process.env,GOWORK:'off',GOFLAGS:'-mod=readonly'},windowsHide:true,maxBuffer:64<<20};
const modules=parseStream((await exec('go',['list','-m','-json','all'],options)).stdout);
if(modules.some(x=>x.Replace))throw Error('Module replacements are prohibited');
for(const[name,version,sum]of [
 ['github.com/MalenkiySolovey/solovey-host','v0.10.0','h1:jZePH9C8Pq/ziw4OI6HdUr/6iuAPN5KiLZcdGplLcj8='],
 ['github.com/MalenkiySolovey/solovey-contracts','v1.0.0','h1:me6uddB3lN19zfYkGp6iuOT46LOshbSelIECAt4Y7aY=']]){
 const selectedModule=modules.find(x=>x.Path===name);
 if(selectedModule?.Version!==version||selectedModule.Sum!==sum)throw Error('Shared dependency identity differs: '+name);
}
const packages=parseStream((await exec('go',['list','-deps','-test','-json','./...'],options)).stdout);
for(const pkg of packages){
 if(pkg.Error||pkg.DepsErrors)throw Error('Package closure error');
 if(pkg.ImportPath?.startsWith('github.com/MalenkiySolovey/solovey-ui')||pkg.ImportPath?.startsWith('github.com/MalenkiySolovey/solovey-node'))throw Error('Private owner in package closure');
 if(pkg.Module&&!pkg.Module.Main&&!pkg.Module.Version)throw Error('Dependency has no immutable version');
}
console.log(JSON.stringify({sourceCommit:provenance.sourceCommit,sourceTree:provenance.sourceTree,files:selected.size,privateProductImports:0,sharedPins:'verified',provenance:'verified'}));
