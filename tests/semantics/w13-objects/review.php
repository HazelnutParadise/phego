<?php
// REQ-012/018/020; T-023/024/035/036/039/040. Review reproductions.
class W13CompareString {public function __toString(){ $GLOBALS['review_events'][]='string';return '42';}}
class W13CompareThrow {public function __toString(){throw new Exception('conversion');}}
$tests['compare-stringable']=function(){ $GLOBALS['review_events']=[];$o=new W13CompareString;return [[$o=='42','42'==$o,$o<=>'42','42'<=>$o],$GLOBALS['review_events']];};
$tests['compare-stringable-throw']=function(){return attempt(fn()=>new W13CompareThrow=='x');};
class W13CompareLazyA {public int $i;}
class W13CompareLazyB {public int $i;}
$tests['compare-lazy-different-classes']=function(){
    $GLOBALS['review_events']=[];
    $a=(new ReflectionClass(W13CompareLazyA::class))->newLazyGhost(function($o){$GLOBALS['review_events'][]='A.init';$o->i=1;});
    $b=(new ReflectionClass(W13CompareLazyB::class))->newLazyGhost(function($o){$GLOBALS['review_events'][]='B.init';$o->i=1;});
    return [$a==$b,$a<=>$b,$GLOBALS['review_events']];
};
class W13CompareNumeric {}
readonly class W13ReferenceReadonly {}
$tests['reference-readonly-dynamic']=function(){$o=new W13ReferenceReadonly;return attempt(function()use($o){$r=&$o->extra;});};
$tests['reference-enum-dynamic']=function(){$o=W13Enum::A;return attempt(function()use($o){$r=&$o->extra;});};
$tests['compare-object-numeric-notice']=function(){$o=new W13CompareNumeric;return [$o==1,$o<=>1,$o==1.0,$o<=>1.0];};
class W13CompareHeld {
    public int $i;
    public string $label;
    public function __destruct(){$GLOBALS['review_events'][]='dtor:'.$this->label;}
}
$tests['compare-lazy-operand-owners']=function(){
    $GLOBALS['review_events']=[];$r=new ReflectionClass(W13CompareHeld::class);
    $GLOBALS['review_left']=$r->newLazyGhost(function($o){$GLOBALS['review_events'][]='init:A';unset($GLOBALS['review_left'],$GLOBALS['review_right']);$o->i=1;$o->label='A';});
    $GLOBALS['review_right']=$r->newLazyGhost(function($o){$GLOBALS['review_events'][]='init:B';$o->i=1;$o->label='B';});
    // labels differ, so equality is false. Both operands outlive initialization.
    $same=$GLOBALS['review_left']==$GLOBALS['review_right'];
    return [$same,$GLOBALS['review_events']];
};
class W13MagicCleanupArgument {public function __destruct(){ $GLOBALS['review_events'][]='argument';throw new Exception('cleanup');}}
class W13MagicCleanup {public function __call($name,$args){unset($GLOBALS['review_argument']);throw $GLOBALS['review_original'];}}
$tests['magic-cleanup-chain']=function(){
    $GLOBALS['review_events']=[];$GLOBALS['review_original']=new Exception('method');$o=new W13MagicCleanup;$GLOBALS['review_argument']=new W13MagicCleanupArgument;
    try {$result=$o->missing($GLOBALS['review_argument']);}catch(Throwable $e){$result=[$e->getMessage(),$e->getPrevious()?->getMessage()];}
    unset($GLOBALS['review_original']);return [$result,$GLOBALS['review_events']];
};
class W13InvalidReturnObject {public function __destruct(){$GLOBALS['review_events'][]='result';}}
class W13InvalidReturn {public function bad():int{return new W13InvalidReturnObject;}}
$tests['method-invalid-return-owner']=function(){ $GLOBALS['review_events']=[];$o=new W13InvalidReturn;$e=attempt(fn()=>$o->bad());return [$e,$GLOBALS['review_events']];};
class W13ReviewValue {
    public function __construct(public string $name){}
    public function __destruct(){$GLOBALS['review_events'][]=$this->name;throw $GLOBALS['review_errors'][$this->name];}
}
$tests['property-error-chain']=function(){
    $GLOBALS['review_events']=[];$GLOBALS['review_errors']=['a'=>new Exception('a'),'b'=>new Exception('b')];
    $o=new stdClass;$o->a=new W13ReviewValue('a');$o->b=new W13ReviewValue('b');
    try{unset($o);}catch(Throwable $e){$GLOBALS['review_events'][]=['caught',$e->getMessage(),$e->getPrevious()?->getMessage()];}
    $GLOBALS['review_events'][]='after';return $GLOBALS['review_events'];
};
$tests['weak-map-error-chain']=function(){
    $GLOBALS['review_events']=[];$GLOBALS['review_errors']=['a'=>new Exception('a'),'b'=>new Exception('b')];
    $m=new WeakMap;$a=new stdClass;$b=new stdClass;$m[$a]=new W13ReviewValue('a');$m[$b]=new W13ReviewValue('b');
    try{unset($m);}catch(Throwable $e){$GLOBALS['review_events'][]=['caught',$e->getMessage(),$e->getPrevious()?->getMessage()];}
    $GLOBALS['review_events'][]='after';return $GLOBALS['review_events'];
};
$tests['array-error-chain']=function(){
    $GLOBALS['review_events']=[];$GLOBALS['review_errors']=['a'=>new Exception('a'),'b'=>new Exception('b')];
    $a=[new W13ReviewValue('a'),new W13ReviewValue('b')];
    try{unset($a);}catch(Throwable $e){$GLOBALS['review_events'][]=['caught',$e->getMessage(),$e->getPrevious()?->getMessage()];}
    $GLOBALS['review_events'][]='after';return $GLOBALS['review_events'];
};
$tests['weak-map-lone-reference-clone']=function(){
    $m=new WeakMap;$k=new stdClass;$m[$k]=1;$r=&$m[$k];unset($r);$c=clone $m;$a=&$m[$k];$b=&$c[$k];$a=2;return [$a,$b,$m[$k],$c[$k]];
};
class W13WeakReentryValue {public function __destruct(){$GLOBALS['review_events'][]='value';unset($GLOBALS['review_map2']);}}
$tests['weak-map-key-free-reentry']=function(){
    $GLOBALS['review_events']=[];$m1=new WeakMap;$GLOBALS['review_map2']=new WeakMap;$k=new stdClass;$m1[$k]=new W13WeakReentryValue;$GLOBALS['review_map2'][$k]=1;unset($k);
    return [$GLOBALS['review_events'],count($m1),isset($GLOBALS['review_map2'])];
};
#[AllowDynamicProperties] class W13RollbackWrite {public int $i=1;public mixed $child=null;}
class W13RollbackNext {public function __destruct(){$GLOBALS['rollback_events'][]='next';}}
class W13RollbackWriter {
    public function __construct(public $owner,public $next,public bool $dynamic){}
    public function __destruct(){
        $GLOBALS['rollback_events'][]='writer';
        if($this->dynamic)$this->owner->added=$this->next;else $this->owner->child=$this->next;
    }
}
foreach([false,true] as $dynamic){
    $tests['lazy-rollback-write-'.($dynamic?'dynamic':'declared')]=function()use($dynamic){
        $GLOBALS['rollback_events']=[];$GLOBALS['rollback_next']=new W13RollbackNext;
        $calls=0;$failure=new Exception('rollback');$r=new ReflectionClass(W13RollbackWrite::class);
        $o=$r->newLazyGhost(function($o)use(&$calls,$failure,$dynamic){
            if(++$calls===1){$writer=new W13RollbackWriter($o,$GLOBALS['rollback_next'],$dynamic);if($dynamic)$o->temp=$writer;else $o->child=$writer;throw $failure;}
            $o->i=9;
        });
        $first=attempt(fn()=>$o->i);unset($GLOBALS['rollback_next']);$before=$GLOBALS['rollback_events'];
        return [$first,$before,$o->i,isset($o->child),isset($o->added),$GLOBALS['rollback_events']];
    };
}
