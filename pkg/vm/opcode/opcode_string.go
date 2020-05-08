// Code generated by "stringer -type Opcode"; DO NOT EDIT.

package opcode

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[PUSHINT8-0]
	_ = x[PUSHINT16-1]
	_ = x[PUSHINT32-2]
	_ = x[PUSHINT64-3]
	_ = x[PUSHINT128-4]
	_ = x[PUSHINT256-5]
	_ = x[PUSHNULL-11]
	_ = x[PUSHDATA1-12]
	_ = x[PUSHDATA2-13]
	_ = x[PUSHDATA4-14]
	_ = x[PUSHM1-15]
	_ = x[PUSH0-16]
	_ = x[PUSHF-16]
	_ = x[PUSH1-17]
	_ = x[PUSHT-17]
	_ = x[PUSH2-18]
	_ = x[PUSH3-19]
	_ = x[PUSH4-20]
	_ = x[PUSH5-21]
	_ = x[PUSH6-22]
	_ = x[PUSH7-23]
	_ = x[PUSH8-24]
	_ = x[PUSH9-25]
	_ = x[PUSH10-26]
	_ = x[PUSH11-27]
	_ = x[PUSH12-28]
	_ = x[PUSH13-29]
	_ = x[PUSH14-30]
	_ = x[PUSH15-31]
	_ = x[PUSH16-32]
	_ = x[NOP-33]
	_ = x[JMP-34]
	_ = x[JMPL-35]
	_ = x[JMPIF-36]
	_ = x[JMPIFL-37]
	_ = x[JMPIFNOT-38]
	_ = x[JMPIFNOTL-39]
	_ = x[JMPEQ-40]
	_ = x[JMPEQL-41]
	_ = x[JMPNE-42]
	_ = x[JMPNEL-43]
	_ = x[JMPGT-44]
	_ = x[JMPGTL-45]
	_ = x[JMPGE-46]
	_ = x[JMPGEL-47]
	_ = x[JMPLT-48]
	_ = x[JMPLTL-49]
	_ = x[JMPLE-50]
	_ = x[JMPLEL-51]
	_ = x[CALL-52]
	_ = x[CALLL-53]
	_ = x[ABORT-55]
	_ = x[ASSERT-56]
	_ = x[THROW-58]
	_ = x[RET-64]
	_ = x[SYSCALL-65]
	_ = x[DEPTH-67]
	_ = x[DROP-69]
	_ = x[NIP-70]
	_ = x[XDROP-72]
	_ = x[CLEAR-73]
	_ = x[DUP-74]
	_ = x[OVER-75]
	_ = x[PICK-77]
	_ = x[TUCK-78]
	_ = x[SWAP-80]
	_ = x[OLDPUSH1-81]
	_ = x[ROT-81]
	_ = x[ROLL-82]
	_ = x[REVERSE3-83]
	_ = x[REVERSE4-84]
	_ = x[REVERSEN-85]
	_ = x[DUPFROMALTSTACK-106]
	_ = x[TOALTSTACK-107]
	_ = x[FROMALTSTACK-108]
	_ = x[CAT-126]
	_ = x[SUBSTR-127]
	_ = x[LEFT-128]
	_ = x[RIGHT-129]
	_ = x[INVERT-144]
	_ = x[AND-145]
	_ = x[OR-146]
	_ = x[XOR-147]
	_ = x[EQUAL-151]
	_ = x[NOTEQUAL-152]
	_ = x[SIGN-153]
	_ = x[ABS-154]
	_ = x[NEGATE-155]
	_ = x[INC-156]
	_ = x[DEC-157]
	_ = x[ADD-158]
	_ = x[SUB-159]
	_ = x[MUL-160]
	_ = x[DIV-161]
	_ = x[MOD-162]
	_ = x[SHL-168]
	_ = x[SHR-169]
	_ = x[NOT-170]
	_ = x[BOOLAND-171]
	_ = x[BOOLOR-172]
	_ = x[NZ-177]
	_ = x[NUMEQUAL-179]
	_ = x[NUMNOTEQUAL-180]
	_ = x[LT-181]
	_ = x[LTE-182]
	_ = x[GT-183]
	_ = x[GTE-184]
	_ = x[MIN-185]
	_ = x[MAX-186]
	_ = x[WITHIN-187]
	_ = x[PACK-192]
	_ = x[UNPACK-193]
	_ = x[NEWARRAY0-194]
	_ = x[NEWARRAY-195]
	_ = x[NEWARRAYT-196]
	_ = x[NEWSTRUCT0-197]
	_ = x[NEWSTRUCT-198]
	_ = x[NEWMAP-200]
	_ = x[SIZE-202]
	_ = x[HASKEY-203]
	_ = x[KEYS-204]
	_ = x[VALUES-205]
	_ = x[PICKITEM-206]
	_ = x[APPEND-207]
	_ = x[SETITEM-208]
	_ = x[REVERSEITEMS-209]
	_ = x[REMOVE-210]
	_ = x[CLEARITEMS-211]
	_ = x[ISNULL-216]
	_ = x[ISTYPE-217]
	_ = x[CONVERT-219]
}

const _Opcode_name = "PUSHINT8PUSHINT16PUSHINT32PUSHINT64PUSHINT128PUSHINT256PUSHNULLPUSHDATA1PUSHDATA2PUSHDATA4PUSHM1PUSH0PUSH1PUSH2PUSH3PUSH4PUSH5PUSH6PUSH7PUSH8PUSH9PUSH10PUSH11PUSH12PUSH13PUSH14PUSH15PUSH16NOPJMPJMPLJMPIFJMPIFLJMPIFNOTJMPIFNOTLJMPEQJMPEQLJMPNEJMPNELJMPGTJMPGTLJMPGEJMPGELJMPLTJMPLTLJMPLEJMPLELCALLCALLLABORTASSERTTHROWRETSYSCALLDEPTHDROPNIPXDROPCLEARDUPOVERPICKTUCKSWAPOLDPUSH1ROLLREVERSE3REVERSE4REVERSENDUPFROMALTSTACKTOALTSTACKFROMALTSTACKCATSUBSTRLEFTRIGHTINVERTANDORXOREQUALNOTEQUALSIGNABSNEGATEINCDECADDSUBMULDIVMODSHLSHRNOTBOOLANDBOOLORNZNUMEQUALNUMNOTEQUALLTLTEGTGTEMINMAXWITHINPACKUNPACKNEWARRAY0NEWARRAYNEWARRAYTNEWSTRUCT0NEWSTRUCTNEWMAPSIZEHASKEYKEYSVALUESPICKITEMAPPENDSETITEMREVERSEITEMSREMOVECLEARITEMSISNULLISTYPECONVERT"

var _Opcode_map = map[Opcode]string{
	0:   _Opcode_name[0:8],
	1:   _Opcode_name[8:17],
	2:   _Opcode_name[17:26],
	3:   _Opcode_name[26:35],
	4:   _Opcode_name[35:45],
	5:   _Opcode_name[45:55],
	11:  _Opcode_name[55:63],
	12:  _Opcode_name[63:72],
	13:  _Opcode_name[72:81],
	14:  _Opcode_name[81:90],
	15:  _Opcode_name[90:96],
	16:  _Opcode_name[96:101],
	17:  _Opcode_name[101:106],
	18:  _Opcode_name[106:111],
	19:  _Opcode_name[111:116],
	20:  _Opcode_name[116:121],
	21:  _Opcode_name[121:126],
	22:  _Opcode_name[126:131],
	23:  _Opcode_name[131:136],
	24:  _Opcode_name[136:141],
	25:  _Opcode_name[141:146],
	26:  _Opcode_name[146:152],
	27:  _Opcode_name[152:158],
	28:  _Opcode_name[158:164],
	29:  _Opcode_name[164:170],
	30:  _Opcode_name[170:176],
	31:  _Opcode_name[176:182],
	32:  _Opcode_name[182:188],
	33:  _Opcode_name[188:191],
	34:  _Opcode_name[191:194],
	35:  _Opcode_name[194:198],
	36:  _Opcode_name[198:203],
	37:  _Opcode_name[203:209],
	38:  _Opcode_name[209:217],
	39:  _Opcode_name[217:226],
	40:  _Opcode_name[226:231],
	41:  _Opcode_name[231:237],
	42:  _Opcode_name[237:242],
	43:  _Opcode_name[242:248],
	44:  _Opcode_name[248:253],
	45:  _Opcode_name[253:259],
	46:  _Opcode_name[259:264],
	47:  _Opcode_name[264:270],
	48:  _Opcode_name[270:275],
	49:  _Opcode_name[275:281],
	50:  _Opcode_name[281:286],
	51:  _Opcode_name[286:292],
	52:  _Opcode_name[292:296],
	53:  _Opcode_name[296:301],
	55:  _Opcode_name[301:306],
	56:  _Opcode_name[306:312],
	58:  _Opcode_name[312:317],
	64:  _Opcode_name[317:320],
	65:  _Opcode_name[320:327],
	67:  _Opcode_name[327:332],
	69:  _Opcode_name[332:336],
	70:  _Opcode_name[336:339],
	72:  _Opcode_name[339:344],
	73:  _Opcode_name[344:349],
	74:  _Opcode_name[349:352],
	75:  _Opcode_name[352:356],
	77:  _Opcode_name[356:360],
	78:  _Opcode_name[360:364],
	80:  _Opcode_name[364:368],
	81:  _Opcode_name[368:376],
	82:  _Opcode_name[376:380],
	83:  _Opcode_name[380:388],
	84:  _Opcode_name[388:396],
	85:  _Opcode_name[396:404],
	106: _Opcode_name[404:419],
	107: _Opcode_name[419:429],
	108: _Opcode_name[429:441],
	126: _Opcode_name[441:444],
	127: _Opcode_name[444:450],
	128: _Opcode_name[450:454],
	129: _Opcode_name[454:459],
	144: _Opcode_name[459:465],
	145: _Opcode_name[465:468],
	146: _Opcode_name[468:470],
	147: _Opcode_name[470:473],
	151: _Opcode_name[473:478],
	152: _Opcode_name[478:486],
	153: _Opcode_name[486:490],
	154: _Opcode_name[490:493],
	155: _Opcode_name[493:499],
	156: _Opcode_name[499:502],
	157: _Opcode_name[502:505],
	158: _Opcode_name[505:508],
	159: _Opcode_name[508:511],
	160: _Opcode_name[511:514],
	161: _Opcode_name[514:517],
	162: _Opcode_name[517:520],
	168: _Opcode_name[520:523],
	169: _Opcode_name[523:526],
	170: _Opcode_name[526:529],
	171: _Opcode_name[529:536],
	172: _Opcode_name[536:542],
	177: _Opcode_name[542:544],
	179: _Opcode_name[544:552],
	180: _Opcode_name[552:563],
	181: _Opcode_name[563:565],
	182: _Opcode_name[565:568],
	183: _Opcode_name[568:570],
	184: _Opcode_name[570:573],
	185: _Opcode_name[573:576],
	186: _Opcode_name[576:579],
	187: _Opcode_name[579:585],
	192: _Opcode_name[585:589],
	193: _Opcode_name[589:595],
	194: _Opcode_name[595:604],
	195: _Opcode_name[604:612],
	196: _Opcode_name[612:621],
	197: _Opcode_name[621:631],
	198: _Opcode_name[631:640],
	200: _Opcode_name[640:646],
	202: _Opcode_name[646:650],
	203: _Opcode_name[650:656],
	204: _Opcode_name[656:660],
	205: _Opcode_name[660:666],
	206: _Opcode_name[666:674],
	207: _Opcode_name[674:680],
	208: _Opcode_name[680:687],
	209: _Opcode_name[687:699],
	210: _Opcode_name[699:705],
	211: _Opcode_name[705:715],
	216: _Opcode_name[715:721],
	217: _Opcode_name[721:727],
	219: _Opcode_name[727:734],
}

func (i Opcode) String() string {
	if str, ok := _Opcode_map[i]; ok {
		return str
	}
	return "Opcode(" + strconv.FormatInt(int64(i), 10) + ")"
}
