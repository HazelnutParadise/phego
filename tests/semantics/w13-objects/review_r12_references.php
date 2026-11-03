<?php
// REQ-018/020; T-035/036/039/040. Hook reads can produce detached object references.
$tests ??= [];
class W13R12Payload {}
class W13R12RefSetInt {public int $x=1 {set{$this->x=$value;}}}
class W13R12RefSetUninit {public int $x {set{$this->x=$value;}}}
class W13R12RefSetNull {public mixed $x=null {set{$this->x=$value;}}}
class W13R12RefSetArray {public array $x=[] {set{$this->x=$value;}}}
class W13R12RefSetVirtual {public int $anchor=0;public int $x {set{$this->anchor=$value;}}}
class W13R12RefSetObject {public object $x {set{$this->x=$value;}}}
class W13R12RefGetScalar {public int $anchor=0;public int $x {get{global$w13R12RefEvents;$w13R12RefEvents[]='get';return 1;}}}
class W13R12RefGetObject {public int $anchor=0;public object $x {get{global$w13R12RefEvents;$w13R12RefEvents[]='get';return new W13R12Payload;}}}
class W13R12RefGetUninit {public int $x {get{global$w13R12RefEvents;$w13R12RefEvents[]='get';return$this->x;}}}
class W13R12RefGetReference {public int $x=1 {&get{global$w13R12RefEvents;$w13R12RefEvents[]='get';return$this->x;}}}
class W13R12RefInherited extends W13R12RefSetInt {}
class W13R12RefInheritedUninit extends W13R12RefSetUninit {}
class W13R12RefOwn {public int $x=1 {set{global$w13R12RefEvents;$w13R12RefEvents[]='set';$alias=&$this->x;$alias=$value;}}}
class W13R12RefHelper {public int $x=1 {set{global$w13R12RefEvents;$w13R12RefEvents[]='set';$before=$this->x;$this->helper($value);}}function helper($value){$alias=&$this->x;$alias=$value;}}
function w13R12RefAttempt(callable$f):mixed {try{return$f();}catch(Throwable$e){return[$e::class,$e->getMessage()];}}
$w13R12RefClasses=['set-int'=>W13R12RefSetInt::class,'set-uninit'=>W13R12RefSetUninit::class,'set-null'=>W13R12RefSetNull::class,'set-array'=>W13R12RefSetArray::class,'set-virtual'=>W13R12RefSetVirtual::class,'set-object'=>W13R12RefSetObject::class,'get-scalar'=>W13R12RefGetScalar::class,'get-object'=>W13R12RefGetObject::class,'get-uninit'=>W13R12RefGetUninit::class,'get-reference'=>W13R12RefGetReference::class,'inherited'=>W13R12RefInherited::class,'inherited-uninit'=>W13R12RefInheritedUninit::class,'own'=>W13R12RefOwn::class,'helper'=>W13R12RefHelper::class];
$w13R12ReferenceNames=[];
foreach($w13R12RefClasses as$kind=>$class)foreach(['normal','ghost','proxy']as$mode) {
    $name='r12-reference-'.$kind.'-'.$mode;$w13R12ReferenceNames[]=$name;
    $tests[$name]=function()use($kind,$class,$mode) {
        global$w13R12RefEvents;$w13R12RefEvents=[];$reflection=new ReflectionClass($class);
        $factory=function()use($class,$kind){$o=new$class;if($kind==='set-object')$o->x=new W13R12Payload;return$o;};
        if($mode==='normal')$object=$factory();
        elseif($mode==='ghost')$object=$reflection->newLazyGhost(function($o)use($kind){global$w13R12RefEvents;$w13R12RefEvents[]='init';if($kind==='set-object')$o->x=new W13R12Payload;});
        else$object=$reflection->newLazyProxy(function()use($factory){global$w13R12RefEvents;$w13R12RefEvents[]='factory';return$factory();});
        $before=$reflection->isUninitializedLazyObject($object);
        $attempt=w13R12RefAttempt(function()use($object,$kind){global$w13R12RefEvents;if($kind==='own'||$kind==='helper'){$object->x=7;}else{$alias=&$object->x;$w13R12RefEvents[]='bound';$alias=2;}return null;});
        $seen=w13R12RefAttempt(function()use($object){$v=$object->x;return[get_debug_type($v),is_object($v)?null:$v];});
        return[$before,$attempt,$seen,$w13R12RefEvents,$reflection->isUninitializedLazyObject($object)];
    };
}
if(realpath($_SERVER['SCRIPT_FILENAME']??'')===__FILE__) {
    $cases=[];foreach($w13R12ReferenceNames as$name){$diagnostics=[];set_error_handler(function($l,$m)use(&$diagnostics){$diagnostics[]=[$l,$m];return true;});$result=$tests[$name]();restore_error_handler();$cases[$name]=['result'=>$result,'diagnostics'=>$diagnostics];}
    echo json_encode(['php'=>PHP_VERSION,'int_size'=>PHP_INT_SIZE,'cases'=>$cases],JSON_PRETTY_PRINT|JSON_THROW_ON_ERROR),"\n";
}
