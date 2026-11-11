<?php
// REQ-018/020; T-035/036/039/040. Typed source mutation and reference destinations.
class W13R5Reference { public string $a='old'; public string $b='old'; public string $c='old'; public mixed $m='old'; public int|string $u='old'; public string|object $so='old'; }
class W13R5Stringable { public function __toString(): string { $GLOBALS['r5_events'][]='convert'; ($GLOBALS['r5_action'])(); return 'new'; } public function __destruct() { $GLOBALS['r5_events'][]='dtor:'.(isset($GLOBALS['r5_obj']->a)?$GLOBALS['r5_obj']->a:'unset'); } }
class W13R5MutationStringable { public function __toString(): string { $GLOBALS['events5'][]='convert'; ($GLOBALS['action5'])(); return 'new'; } }
class W13R5Mutation { public string $a='old'; public string $b='old'; public string $c='old'; public mixed $m='old'; }
class W13R5IntTarget { public int $i=7; }
function w13R5ReferenceAttempt($f) { try { return $f(); } catch (Throwable $problem) { $r=[]; do { $r[]=[$problem::class,$problem->getMessage()]; $problem=$problem->getPrevious(); } while ($problem); return $r; } }
function w13R5ReferenceProbe($mode) {
    $o=new W13R5Reference; $GLOBALS['r5_obj']=$o; $GLOBALS['r5_events']=[]; $GLOBALS['r5_other']='other';
    if ($mode==='mixed-first') { $GLOBALS['r5_alias']=&$o->m; $o->a=&$GLOBALS['r5_alias']; }
    else { $GLOBALS['r5_alias']=&$o->a; if ($mode==='string-mixed') $o->m=&$GLOBALS['r5_alias']; elseif ($mode==='string-objectunion') $o->so=&$GLOBALS['r5_alias']; elseif ($mode==='string-union') $o->u=&$GLOBALS['r5_alias']; else $o->b=&$GLOBALS['r5_alias']; }
    $GLOBALS['r5_action']=function() use($o,$mode) { switch($mode) { case 'unset-first': unset($o->a); break; case 'unset-next': unset($o->b); break; case 'unset-all': unset($o->a,$o->b); break; case 'add-source': $o->c=&$GLOBALS['r5_alias']; break; case 'replace-source': unset($o->b); $o->b=&$GLOBALS['r5_other']; break; case 'unset-alias': unset($GLOBALS['r5_alias']); break; case 'rebind-alias': $GLOBALS['r5_alias']=&$GLOBALS['r5_other']; break; case 'drop-rhs': unset($GLOBALS['r5_rhs']); break; } };
    $GLOBALS['r5_rhs']=new W13R5Stringable;
    $assignment=w13R5ReferenceAttempt(function(){ $GLOBALS['r5_alias']=$GLOBALS['r5_rhs']; return 'ok'; });
    $props=[]; foreach(['a','b','c','m','u','so'] as $n) $props[$n]=isset($o->$n)?$o->$n:'unset';
    $result=[$assignment,$props,array_key_exists('r5_alias',$GLOBALS)?$GLOBALS['r5_alias']:'unset',$GLOBALS['r5_other'],$GLOBALS['r5_events']];
    unset($GLOBALS['r5_rhs'],$GLOBALS['r5_alias'],$GLOBALS['r5_other'],$GLOBALS['r5_action'],$GLOBALS['r5_obj']); return $result;
}
function w13R5MutationProbe($mode) {
    $o=new W13R5Mutation; $GLOBALS['events5']=[]; $GLOBALS['alias5']=&$o->a; $o->b=&$GLOBALS['alias5']; $target=new W13R5IntTarget;
    if (str_starts_with($mode,'triple-')) $o->m=&$GLOBALS['alias5'];
    if ($mode==='duplicate-clone') { unset($o->b); $clone=clone $o; $o->m=&$GLOBALS['alias5']; unset($clone->a); }
    $calls=0; $GLOBALS['action5']=function() use($o,$target,$mode,&$calls) { if (++$calls!==1) return; switch($mode) { case 'triple-unset-first': unset($o->a); break; case 'triple-unset-next': unset($o->b); break; case 'triple-unset-last': unset($o->m); break; case 'triple-replace-last': unset($o->m); $o->c=&$GLOBALS['alias5']; break; case 'append-remove': $o->m=&$GLOBALS['alias5']; unset($o->b); break; case 'remove-append': unset($o->b); $o->m=&$GLOBALS['alias5']; break; case 'rebind-typed': $GLOBALS['alias5']=&$target->i; break; } };
    $assigned=w13R5ReferenceAttempt(function(){ $GLOBALS['alias5']=new W13R5MutationStringable; return 'ok'; }); $props=[]; foreach(['a','b','c','m'] as $n) $props[$n]=isset($o->$n)?$o->$n:'unset';
    $out=[$assigned,$props,$GLOBALS['alias5'],$target->i,$GLOBALS['events5']];
    if ($mode==='duplicate-clone') { unset($o->a); $out[]=w13R5ReferenceAttempt(function(){ $GLOBALS['alias5']=new stdClass; return 'ok'; }); $out[]=get_debug_type($GLOBALS['alias5']); }
    unset($GLOBALS['action5'],$GLOBALS['alias5']); return $out;
}
foreach (['control','unset-first','unset-next','unset-all','add-source','replace-source','unset-alias','rebind-alias','drop-rhs','string-mixed','mixed-first','string-objectunion','string-union'] as $mode) $tests['r5-ref-'.$mode]=fn()=>w13R5ReferenceProbe($mode);
foreach (['triple-unset-first','triple-unset-next','triple-unset-last','triple-replace-last','append-remove','remove-append','duplicate-clone','rebind-typed'] as $mode) $tests['r5-ref-'.$mode]=fn()=>w13R5MutationProbe($mode);
