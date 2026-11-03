<?php
// REQ-018; T-035/036. Trait composition precedes ordinary inheritance.
$tests ??= [];
foreach (['public','protected'] as $access) foreach ([false,true] as $static) foreach ([false,true] as $two) {
    $suffix=$access.'_'.($static?'static':'instance').'_'.($two?'two':'one');
    $parent='W13R11Parent_'.$suffix; $child='W13R11Child_'.$suffix; $plain='W13R11Plain_'.$suffix;
    $a='W13R11ImportA_'.$suffix; $b='W13R11ImportB_'.$suffix; $flag=$static?'static ':'';
    eval('class '.$parent.' {'.$access.' '.$flag.'int $x=9;}');
    eval('trait '.$a.' {'.$access.' '.$flag.'int $x=2;}');
    eval('trait '.$b.' {'.$access.' '.$flag.'int $x=2;}');
    eval('class '.$child.' extends '.$parent.' {use '.$a.($two?','.$b:'').';}');
    eval('class '.$plain.' extends '.$parent.' {}');
    $tests['r11-trait-inherit-'.$suffix]=function()use($parent,$child,$plain,$static) {
        $p=new ReflectionProperty($parent,'x');$c=new ReflectionProperty($child,'x');$d=new ReflectionProperty($plain,'x');
        $po=$static?null:new $parent;$co=$static?null:new $child;$do=$static?null:new $plain;
        $before=[$p->getValue($po),$c->getValue($co),$d->getValue($do)];
        $p->setValue($po,10);$c->setValue($co,20);
        return [[$c->getDeclaringClass()->getName(),$d->getDeclaringClass()->getName()],$before,[$p->getValue($po),$c->getValue($co),$d->getValue($do)]];
    };
}
trait W13R11IterableA {public iterable $x=[];}
trait W13R11IterableB {public Traversable|array $x=[];}
class W13R11Iterable {use W13R11IterableA,W13R11IterableB;}
foreach(['normal','lazy']as$mode)$tests['r11-trait-iterable-'.$mode]=function()use($mode) {
    $r=new ReflectionClass(W13R11Iterable::class);$calls=0;
    $o=$mode==='lazy'?$r->newLazyGhost(function()use(&$calls){++$calls;}):new W13R11Iterable;
    $before=[$r->isUninitializedLazyObject($o),$calls];
    $r->getProperty('x')->setRawValueWithoutLazyInitialization($o,[2]);
    return [$before,[$r->isUninitializedLazyObject($o),$calls],$o->x];
};
foreach([['protected','private'],['private','protected'],['public','protected']]as[$first,$second]) {
    $class='W13R11Setter_'.$first.'_'.$second;$a=$class.'A';$b=$class.'B';
    eval('trait '.$a.' {public '.$first.'(set) int $x=1;}');eval('trait '.$b.' {public '.$second.'(set) int $x=1;}');eval('class '.$class.' {use '.$a.','.$b.';}');
    $tests['r11-trait-setter-'.$first.'-'.$second]=function()use($class) {
        $o=new $class;$p=new ReflectionProperty($class,'x');$result=[$p->isPrivateSet(),$p->isProtectedSet()];
        try{$o->x=2;$result[]=null;}catch(Throwable$e){$result[]=[$e::class,$e->getMessage()];}
        $result[]=$o->x;return$result;
    };
}
if(realpath($_SERVER['SCRIPT_FILENAME']??'')===__FILE__) {
    $cases=[];foreach($tests as$name=>$test){$diagnostics=[];set_error_handler(function($l,$m)use(&$diagnostics){$diagnostics[]=[$l,$m];return true;});$result=$test();restore_error_handler();$cases[$name]=['result'=>$result,'diagnostics'=>$diagnostics];}
    echo json_encode(['php'=>PHP_VERSION,'int_size'=>PHP_INT_SIZE,'cases'=>$cases],JSON_PRETTY_PRINT|JSON_THROW_ON_ERROR),"\n";
}
