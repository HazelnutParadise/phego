<?php
// REQ-018/020; T-035/036/039/040. Preserve coercion exceptions and operand cleanup.
class W13R5Life { public string $a='old'; public string $b='old'; }
class W13R5Throw { function __toString():string { $GLOBALS['r5_life_events'][]='convert'; throw new Exception('convert'); } }
class W13R5Old { function __destruct() { $GLOBALS['r5_life_events'][]='old'; } }
class W13R5Array { public array|string $a; public array|string $b; }
class W13R5LifeString { function __toString():string { $GLOBALS['r5_life_events'][]='convert'; unset($GLOBALS['r5_life_rhs']); return 'new'; } function __destruct() { $GLOBALS['r5_life_events'][]='rhs'; } }
foreach ([1,2] as $n) $tests['r5-life-throw-'.$n]=function()use($n) {
    $o=new W13R5Life; $r=&$o->a; if($n===2)$o->b=&$r;
    $GLOBALS['r5_life_events']=[]; $x=new W13R5Throw;
    $result=w13R5ReferenceAttempt(function()use(&$r,$x){$r=$x;return 'ok';});
    return [$result,$r,$GLOBALS['r5_life_events']];
};
$tests['r5-life-cleanup-order']=function() {
    $o=new W13R5Array; $o->a=[new W13R5Old]; $r=&$o->a; $o->b=&$r;
    $GLOBALS['r5_life_events']=[]; $GLOBALS['r5_life_rhs']=new W13R5LifeString;
    $result=w13R5ReferenceAttempt(function()use(&$r){$r=$GLOBALS['r5_life_rhs'];return 'ok';});
    return [$result,$r,$GLOBALS['r5_life_events']];
};
class W13R5Self { public ?self $p=null; }
class W13R5ParentBase {}
class W13R5Parent extends W13R5ParentBase { public ?parent $p=null; }
$tests['r5-life-self']=function() {
    $o=new W13R5Self; $r=&$o->p;
    $result=w13R5ReferenceAttempt(function()use(&$r,$o){$r=$o;return 'ok';});
    $out=[$result,$r===$o]; $r=null; return $out;
};
$tests['r5-life-parent']=function() {
    $o=new W13R5Parent; $r=&$o->p; $target=new W13R5ParentBase;
    $result=w13R5ReferenceAttempt(function()use(&$r,$target){$r=$target;return 'ok';});
    return [$result,$r===$target];
};
class W13R5MixedScalar { public mixed $m=1; public int $i=1; }
foreach ([false,true] as $mixed) $tests[$mixed?'r5-life-mixed-array':'r5-life-invalid-array']=function()use($mixed) {
    $o=new W13R5MixedScalar; if($mixed){$r=&$o->m;$o->i=&$r;}else{$r=&$o->i;}
    return w13R5ReferenceAttempt(function()use(&$r){$r=[];});
};
class W13R5InfoBase {public string $a='old';public mixed $m='old';}
class W13R5InfoChild extends W13R5InfoBase {}
class W13R5InfoRedeclared extends W13R5InfoBase {public string $a='old';}
class W13R5PrivateBase {private string $a='old';public mixed $m='old';function &alias(){return $this->a;}function clear(){unset($this->a);}}
class W13R5PrivateChild extends W13R5PrivateBase {private string $a='old';function bind(&$r){$this->a=&$r;}function clearChild(){unset($this->a);}}
trait W13R5InfoTrait {public string $a='old';public mixed $m='old';}
class W13R5TraitA {use W13R5InfoTrait;}
class W13R5TraitB {use W13R5InfoTrait;}
class W13R5InfoStringable {function __toString():string{$GLOBALS['r5_info_events'][]='convert';return 'new';}}
foreach(['inherited','redeclared','private','trait']as$mode)$tests['r5-life-source-'.$mode]=function()use($mode){
    if($mode==='private'){$p=new W13R5PrivateBase;$c=new W13R5PrivateChild;$r=&$p->alias();$c->bind($r);}
    else{$p=$mode==='trait'?new W13R5TraitA:new W13R5InfoBase;$c=match($mode){'inherited'=>new W13R5InfoChild,'redeclared'=>new W13R5InfoRedeclared,'trait'=>new W13R5TraitB};$r=&$p->a;$c->a=&$r;}
    $p->m=&$r;if($mode==='private')$c->clearChild();else unset($c->a);
    $GLOBALS['r5_info_events']=[];$first=w13R5ReferenceAttempt(function()use(&$r){$r=new W13R5InfoStringable;});$out=[$first,$GLOBALS['r5_info_events']];
    if($mode==='private')$p->clear();else unset($p->a);
    $out[]=w13R5ReferenceAttempt(function()use(&$r){$r=new stdClass;});$out[]=get_debug_type($r);return$out;
};
