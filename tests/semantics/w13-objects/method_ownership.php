<?php
// REQ-012/020; T-023/024/039/040. Operands and formal parameters have distinct owners.
class W13OwnedParameter {
    public function __construct(public string $name,public bool $fail=false){}
    public function __destruct(){$GLOBALS['owned_events'][]=$this->name;if($this->fail)throw new Exception('cleanup');}
}
class W13OwnedString {
    public function __construct(public string $remove){}
    public function __toString():string{$GLOBALS['owned_events'][]='string';unset($GLOBALS[$this->remove]);return 'abc';}
    public function __destruct(){$GLOBALS['owned_events'][]='string-dtor';}
}
class W13OwnershipMethods {
    public function order($a,$b){unset($GLOBALS['owned_a'],$GLOBALS['owned_b']);$GLOBALS['owned_events'][]='body';return 7;}
    public function text(string $s){$GLOBALS['owned_events'][]='body';}
    public function second(string $s,$p){$GLOBALS['owned_events'][]=is_object($p)?'body':'dead';}
    public function fail($p){unset($GLOBALS['owned_a']);return new W13MethodResult;}
    public function __destruct(){$GLOBALS['owned_events'][]='receiver';}
}
foreach(['parameter-order','named-parameter-order','coerce-receiver','coerce-second','coerce-source','failed-parameter'] as $case){
    $tests['method-'.$case]=function()use($case){
        $GLOBALS['owned_events']=[];$GLOBALS['method_events']=&$GLOBALS['owned_events'];
        $m=new W13OwnershipMethods;
        if($case==='parameter-order'||$case==='named-parameter-order'){
            $GLOBALS['owned_a']=new W13OwnedParameter('a');$GLOBALS['owned_b']=new W13OwnedParameter('b');
            if($case==='parameter-order')$r=$m->order($GLOBALS['owned_a'],$GLOBALS['owned_b']);else $r=$m->order(b:$GLOBALS['owned_b'],a:$GLOBALS['owned_a']);
        }elseif($case==='coerce-receiver'){
            $GLOBALS['owned_receiver']=$m;unset($m);$s=new W13OwnedString('owned_receiver');$GLOBALS['owned_receiver']->text($s);
        }elseif($case==='coerce-second'){
            $GLOBALS['owned_a']=new W13OwnedParameter('arg2');$s=new W13OwnedString('owned_a');$m->second($s,$GLOBALS['owned_a']);
        }elseif($case==='coerce-source'){
            $GLOBALS['owned_source']=new W13OwnedString('owned_source');$m->text($GLOBALS['owned_source']);
        }else{
            $GLOBALS['owned_a']=new W13OwnedParameter('parameter',true);
            try{$r=$m->fail($GLOBALS['owned_a']);}catch(Throwable $e){$GLOBALS['owned_events'][]='caught:'.$e->getMessage();}
        }
        $GLOBALS['owned_events'][]='after';return $GLOBALS['owned_events'];
    };
}
