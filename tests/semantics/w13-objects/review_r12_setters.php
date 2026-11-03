<?php
// REQ-018/020; T-035/036/039/040. Setter parameters are distinct from backing types.
$tests ??= [];
require_once __DIR__.'/review_r12_setter_strict.php';
class W13R12SetterText {function __toString():string {global$w13R12SetterEvents;$w13R12SetterEvents[]='convert';return'abc';}}
class W13R12SetterBackedWide {public int $stored=0;public int $x=0 {set(int|string $incoming){global$w13R12SetterEvents;$w13R12SetterEvents[]=['body',get_debug_type($incoming),$incoming];$n=is_string($incoming)?strlen($incoming):$incoming;$this->x=$n;$this->stored=$n;}}}
class W13R12SetterVirtualWide {public int $stored=0;public int $x {set(int|string $incoming){global$w13R12SetterEvents;$w13R12SetterEvents[]=['body',get_debug_type($incoming),$incoming];$this->stored=is_string($incoming)?strlen($incoming):$incoming;}}}
class W13R12SetterImplicit {public int $stored=0;public int $x=0 {set{global$w13R12SetterEvents;$w13R12SetterEvents[]=['body',get_debug_type($value),$value];$this->x=$value;$this->stored=$value;}}}
class W13R12SetterExplicit {public int $stored=0;public int $x=0 {set(int $incoming){global$w13R12SetterEvents;$w13R12SetterEvents[]=['body',get_debug_type($incoming),$incoming];$this->x=$incoming;$this->stored=$incoming;}}}
function w13R12LooseSetter($object,$value):void {$object->x=$value;}
function w13R12ObserveSetter($class,$input,$strict,$mode) {
        global$w13R12SetterEvents;$w13R12SetterEvents=[];$reflection=new ReflectionClass($class);
        if($mode==='normal')$o=new$class;
        elseif($mode==='ghost')$o=$reflection->newLazyGhost(function(){global$w13R12SetterEvents;$w13R12SetterEvents[]='init';});
        else$o=$reflection->newLazyProxy(function()use($class){global$w13R12SetterEvents;$w13R12SetterEvents[]='factory';return new$class;});
        $v=match($input){'text'=>'abc','numeric'=>'12','int'=>2,'float'=>2.5,'bool'=>true,'null'=>null,'array'=>[],default=>new W13R12SetterText};
        try{if($strict==='strict')w13R12StrictSetter($o,$v);else w13R12LooseSetter($o,$v);$error=null;}catch(Throwable$e){$error=[$e::class,$e->getMessage()];}
        $afterAssign=$reflection->isUninitializedLazyObject($o);$assignEvents=$w13R12SetterEvents;$stored=$o->stored;
        return[$error,$afterAssign,$assignEvents,$stored,$w13R12SetterEvents,$reflection->isUninitializedLazyObject($o)];
}
$w13R12SetterClasses=['backed-wide'=>W13R12SetterBackedWide::class,'virtual-wide'=>W13R12SetterVirtualWide::class,'implicit'=>W13R12SetterImplicit::class,'explicit'=>W13R12SetterExplicit::class];
$w13R12SetterNames=[];
foreach($w13R12SetterClasses as$kind=>$class)foreach(['text','numeric','int','float','bool','null','array','object']as$input)foreach(['weak','strict']as$strict)foreach(['normal','ghost','proxy']as$mode) {
    $name='r12-setter-'.$kind.'-'.$input.'-'.$strict.'-'.$mode;$w13R12SetterNames[]=$name;
    $tests[$name]=fn()=>w13R12ObserveSetter($class,$input,$strict,$mode);
}
class W13R12SetterInherited extends W13R12SetterBackedWide {}
trait W13R12SetterTrait {public int $stored=0;public int $x=0 {set(int|string $traitInput){global$w13R12SetterEvents;$w13R12SetterEvents[]=['body',get_debug_type($traitInput),$traitInput];$n=is_string($traitInput)?strlen($traitInput):$traitInput;$this->x=$n;$this->stored=$n;}}}
class W13R12SetterComposed {use W13R12SetterTrait;}
class W13R12SetterOverride extends W13R12SetterBackedWide {public int $x=0 {set(int|string $renamed){global$w13R12SetterEvents;$w13R12SetterEvents[]=['body',get_debug_type($renamed),$renamed];$n=is_string($renamed)?strlen($renamed):$renamed;$this->x=$n;$this->stored=$n;}}}
foreach(['inherited'=>W13R12SetterInherited::class,'trait'=>W13R12SetterComposed::class,'override'=>W13R12SetterOverride::class]as$kind=>$class)foreach(['text','array']as$input)foreach(['normal','ghost','proxy']as$mode){
    $name='r12-setter-metadata-'.$kind.'-'.$input.'-weak-'.$mode;$w13R12SetterNames[]=$name;
    $tests[$name]=fn()=>w13R12ObserveSetter($class,$input,'weak',$mode);
}
if(realpath($_SERVER['SCRIPT_FILENAME']??'')===__FILE__) {
    $cases=[];foreach($w13R12SetterNames as$name){$diagnostics=[];set_error_handler(function($l,$m)use(&$diagnostics){$diagnostics[]=[$l,$m];return true;});$result=$tests[$name]();restore_error_handler();$cases[$name]=['result'=>$result,'diagnostics'=>$diagnostics];}
    echo json_encode(['php'=>PHP_VERSION,'int_size'=>PHP_INT_SIZE,'cases'=>$cases],JSON_PRETTY_PRINT|JSON_THROW_ON_ERROR),"\n";
}
