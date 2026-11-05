#!/usr/bin/env bash
# Перевіряє задеплоєну мережу на відповідність specs/network/spec.md. Код виходу 0 = усе добре.
#   bash scripts/verify-network.sh
set -uo pipefail
STACK="${STACK:-kv-network}"
REGION="${AWS_REGION:-us-east-1}"
FAIL=0
ok()   { echo "  OK   $*"; }
bad()  { echo "  FAIL $*"; FAIL=1; }
out()  { aws cloudformation describe-stacks --stack-name "$STACK" --region "$REGION" \
           --query "Stacks[0].Outputs[?OutputKey=='$1'].OutputValue" --output text; }

VPC=$(out VpcId); PUB_RT=$(out PublicRouteTableId); PRIV_RT=$(out PrivateRouteTableId)
PUBS=$(out PublicSubnetIds | tr ',' ' '); PRIVS=$(out PrivateSubnetIds | tr ',' ' ')
echo "VPC $VPC"

CIDR=$(aws ec2 describe-vpcs --vpc-ids "$VPC" --region "$REGION" --query 'Vpcs[0].CidrBlock' --output text)
[ "$CIDR" = "10.20.0.0/16" ] && ok "VPC CIDR $CIDR" || bad "VPC CIDR is $CIDR"

AZS=$(aws ec2 describe-subnets --subnet-ids $PUBS $PRIVS --region "$REGION" \
        --query 'Subnets[].AvailabilityZone' --output text | tr '\t' '\n' | sort -u | wc -l)
[ "$AZS" -eq 2 ] && ok "subnets span 2 AZs" || bad "subnets span $AZS AZs"

PUB_IGW=$(aws ec2 describe-route-tables --route-table-ids "$PUB_RT" --region "$REGION" \
  --query "RouteTables[0].Routes[?DestinationCidrBlock=='0.0.0.0/0'].GatewayId" --output text)
[[ "$PUB_IGW" == igw-* ]] && ok "public RT: 0.0.0.0/0 -> $PUB_IGW" || bad "public RT has no IGW route"

PRIV_DEF=$(aws ec2 describe-route-tables --route-table-ids "$PRIV_RT" --region "$REGION" \
  --query "RouteTables[0].Routes[?DestinationCidrBlock=='0.0.0.0/0']" --output text)
[ -z "$PRIV_DEF" ] && ok "private RT: no route to the internet" || bad "private RT has a default route"

# кожна підмережа має бути прив'язана саме до своєї таблиці маршрутів
assoc() { aws ec2 describe-route-tables --route-table-ids "$1" --region "$REGION" \
  --query 'RouteTables[0].Associations[].SubnetId' --output text; }
PUB_ASSOC=$(assoc "$PUB_RT"); PRIV_ASSOC=$(assoc "$PRIV_RT")
for s in $PUBS; do
  [[ "$PUB_ASSOC" == *"$s"* ]] && ok "$s uses the public RT" || bad "$s is not on the public RT"
done
for s in $PRIVS; do
  [[ "$PRIV_ASSOC" == *"$s"* ]] && ok "$s uses the private RT" || bad "$s is not on the private RT"
done

for s in $PRIVS; do
  MAP=$(aws ec2 describe-subnets --subnet-ids "$s" --region "$REGION" --query 'Subnets[0].MapPublicIpOnLaunch' --output text)
  [ "$MAP" = "False" ] && ok "$s does not assign public IPs" || bad "$s assigns public IPs"
done

EDGE_SG=$(out EdgeSecurityGroupId); COORD_SG=$(out CoordinatorSecurityGroupId); SHARD_SG=$(out ShardSecurityGroupId)
param() { aws cloudformation describe-stacks --stack-name "$STACK" --region "$REGION" \
  --query "Stacks[0].Parameters[?ParameterKey=='$1'].ParameterValue" --output text; }
COORD_PORT=$(param CoordinatorPort); SHARD_PORT=$(param ShardPort)

# усі вхідні правила групи, один рядок на правило: протокол, порти, джерела
rules() { aws ec2 describe-security-groups --group-ids "$1" --region "$REGION" \
  --query 'SecurityGroups[0].IpPermissions[].[IpProtocol, FromPort, ToPort, join(`,`, UserIdGroupPairs[].GroupId), join(`,`, IpRanges[].CidrIp), join(`,`, Ipv6Ranges[].CidrIpv6), join(`,`, PrefixListIds[].PrefixListId)]' \
  --output text | awk '{$1=$1};1'; }
# правило має бути рівно одне і точно таке, як очікується: зайве правило теж провалює перевірку
only() { local got; got=$(rules "$2")
  [ "$got" = "$3" ] && ok "$1: only $3" || bad "$1 rules: $got"; }

only edge-sg "$EDGE_SG" "tcp 80 80 0.0.0.0/0"
only coordinator-sg "$COORD_SG" "tcp $COORD_PORT $COORD_PORT $EDGE_SG"
only shard-sg "$SHARD_SG" "tcp $SHARD_PORT $SHARD_PORT $COORD_SG"

# порт 22 може бути відкритий і діапазоном, і правилом на весь трафік
SSH=$(aws ec2 describe-security-groups --group-ids "$EDGE_SG" "$COORD_SG" "$SHARD_SG" --region "$REGION" \
  --query "SecurityGroups[].IpPermissions[?IpProtocol=='-1' || (FromPort<=\`22\` && ToPort>=\`22\`)][]" --output text)
[ -z "$SSH" ] && ok "no security group opens port 22" || bad "port 22 is open"

[ $FAIL -eq 0 ] && echo "ALL CHECKS PASSED" || echo "SOME CHECKS FAILED"
exit $FAIL
